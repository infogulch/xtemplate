// Package git is a ServerController that serves templates from a git repository
// via the installed `git` binary. JSON type: "git".
package git

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/infogulch/xtemplate"
	"github.com/spf13/afero"
)

func init() {
	xtemplate.RegisterController("git", func() xtemplate.ServerController { return &Controller{} })
}

// Controller polls a remote (or local) git repo and reloads when the ref moves.
// JSON type: "git".
type Controller struct {
	// Repo is the git repository URL or path. Required.
	Repo string `json:"repo,omitempty" arg:"--git-repo"`

	// Ref is the branch/tag/ref to track. Empty uses the remote default.
	Ref string `json:"ref,omitempty" arg:"--git-ref"`

	// Interval between polls. Default 15s.
	Interval xtemplate.Duration `json:"interval,omitempty" arg:"--git-interval"`

	// Path is the subdirectory inside the clone that holds templates. Default "templates".
	// Must be a relative path with no ".." segments (confined to the clone).
	Path string `json:"path,omitempty" arg:"-t,--template-dir,--templates-dir" default:"templates"`
}

// Init validates config. Sticky is nil until the first successful Reload.
func (s *Controller) Init(_ context.Context, _ *slog.Logger) ([]xtemplate.Option, error) {
	if s.Repo == "" {
		return nil, fmt.Errorf("xtemplate: git controller missing repo (--git-repo or controller.repo)")
	}
	if err := validateGitArg("repo", s.Repo); err != nil {
		return nil, err
	}
	if s.Ref != "" {
		if err := validateGitArg("ref", s.Ref); err != nil {
			return nil, err
		}
	}
	sub := s.Path
	if sub == "" {
		sub = "templates"
	}
	if _, err := confinedSubdir("", sub); err != nil {
		return nil, err
	}
	return nil, nil
}

// Start polls the remote and reloads on new commits. Cancelling server.Context()
// stops the poller. last-SHA advances only when Reload succeeds.
func (s *Controller) Start(server *xtemplate.Server) error {
	log := server.Logger()
	if log == nil {
		log = slog.Default()
	}
	log = log.WithGroup("controller")
	interval := s.Interval.Duration()
	if interval == 0 {
		interval = 15 * time.Second
	}
	sub := s.Path
	if sub == "" {
		sub = "templates"
	}
	go s.poll(server.Context(), log, interval, sub, server)
	return nil
}

func (s *Controller) poll(ctx context.Context, log *slog.Logger, interval time.Duration, sub string, srv *xtemplate.Server) {
	var last string
	check := func() {
		if ctx.Err() != nil {
			return
		}
		sha, err := lsRemote(ctx, s.Repo, s.Ref)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Warn("ls-remote failed", slog.Any("error", err))
			return
		}
		if sha == last {
			return
		}
		newdir, err := clone(ctx, s.Repo, s.Ref)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Warn("clone failed", slog.Any("error", err))
			return
		}
		fsys, err := subFS(newdir, sub)
		if err != nil {
			_ = os.RemoveAll(newdir)
			log.Warn("template path rejected", slog.Any("error", err))
			return
		}
		if ctx.Err() != nil {
			_ = os.RemoveAll(newdir)
			return
		}
		log.Info("reloading from new commit", slog.String("commit", sha), slog.String("dir", newdir))
		err = srv.Reload(
			xtemplate.WithTemplateFS(fsys),
			xtemplate.WithOnClose(func() error { return os.RemoveAll(newdir) }),
		)
		if err == nil {
			last = sha
		}
	}

	check()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

// validateGitArg rejects empty values and leading dashes (argv option injection).
func validateGitArg(name, val string) error {
	if val == "" {
		return fmt.Errorf("xtemplate: git controller %s is empty", name)
	}
	if strings.HasPrefix(val, "-") {
		return fmt.Errorf("xtemplate: git controller %s must not start with '-'", name)
	}
	return nil
}

// userinfoRedact strips password (and leaves username) in URL userinfo for logs.
var userinfoRedact = regexp.MustCompile(`(//[^:/@\s]+):([^@/\s]+)@`)

func redactSecrets(s string) string {
	return userinfoRedact.ReplaceAllString(s, `${1}:***@`)
}

// confinedSubdir resolves subdir under cloneDir and ensures it stays inside the clone.
// cloneDir may be empty to validate shape only (relative, no "..").
func confinedSubdir(cloneDir, subdir string) (string, error) {
	if subdir == "" {
		subdir = "templates"
	}
	if filepath.IsAbs(subdir) {
		return "", fmt.Errorf("xtemplate: git controller path must be relative, got %q", subdir)
	}
	clean := filepath.Clean(subdir)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("xtemplate: git controller path escapes clone: %q", subdir)
	}
	if cloneDir == "" {
		return clean, nil
	}
	root := filepath.Join(cloneDir, clean)
	absClone, err := filepath.Abs(cloneDir)
	if err != nil {
		return "", err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	sep := string(filepath.Separator)
	if absRoot != absClone && !strings.HasPrefix(absRoot, absClone+sep) {
		return "", fmt.Errorf("xtemplate: git controller path escapes clone: %q", subdir)
	}
	return absRoot, nil
}

func subFS(cloneDir, subdir string) (afero.Fs, error) {
	root, err := confinedSubdir(cloneDir, subdir)
	if err != nil {
		return nil, err
	}
	if err := rejectSymlinks(root); err != nil {
		return nil, err
	}
	return afero.NewBasePathFs(afero.NewOsFs(), root), nil
}

// rejectSymlinks walks root and errors if any symlink is present.
// afero.BasePathFs follows symlinks on Open, which would let a malicious repo
// expose host files as static content.
func rejectSymlinks(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			rel, _ := filepath.Rel(root, path)
			return fmt.Errorf("xtemplate: git controller refuses symlink %q (afero BasePathFs follows links)", rel)
		}
		return nil
	})
}

// clone makes a shallow clone of url@ref into a fresh temp dir and returns its
// path. An empty ref clones the repository's default branch.
func clone(ctx context.Context, url, ref string) (string, error) {
	dir, err := os.MkdirTemp("", "xtemplate-git-")
	if err != nil {
		return "", err
	}
	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, "--", url, dir)
	if out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput(); err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("git clone: %w: %s", err, redactSecrets(string(out)))
	}
	return dir, nil
}

// lsRemote returns the commit SHA that ref points to on the remote without
// cloning. An empty ref resolves the remote's HEAD.
func lsRemote(ctx context.Context, url, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	out, err := exec.CommandContext(ctx, "git", "ls-remote", "--", url, ref).Output()
	if err != nil {
		// Prefer not to dump remote stderr (may include credentials in URL).
		return "", fmt.Errorf("git ls-remote: %w", err)
	}
	return parseLsRemote(string(out))
}

// parseLsRemote returns the SHA from the first line of `git ls-remote` output,
// where each line is "<sha>\t<refname>".
func parseLsRemote(out string) (string, error) {
	out = strings.TrimSpace(out)
	if out == "" {
		return "", fmt.Errorf("ref not found on remote")
	}
	line, _, _ := strings.Cut(out, "\n")
	sha, _, ok := strings.Cut(line, "\t")
	if !ok || sha == "" {
		return "", fmt.Errorf("malformed ls-remote line: %q", line)
	}
	return sha, nil
}
