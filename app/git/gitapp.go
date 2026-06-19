// Package git serves an xtemplate site from a git repository, reusing app's
// config loading machinery. The server starts immediately with an empty FS and
// begins serving once the local git repo is cloned and ready.
//
// Design note: this implementation shells out to installed `git` binary instead
// of adding an application dependency, and re-clones on every change instead of
// managing worktrees. Switch to go-git / worktrees if cold-clone latency or
// disk churn ever matters.
package git

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/app"
	"github.com/spf13/afero"
)

type gitConfig struct {
	GitRepo     string        `json:"git_repo" arg:"--git-repo"`
	GitRef      string        `json:"git_ref" arg:"--git-ref"`
	GitInterval time.Duration `json:"git_interval" arg:"--git-interval"`
}

// Config extends the default xtemplate cli with flags to configure the git source.
type Config struct {
	app.Config
	gitConfig
}

func (a *Config) SetDefaults() {
	a.Config.SetDefaults()
	a.GitInterval = 15 * time.Second
}

var _ app.Configurable = &Config{}

func Main(overrides ...xtemplate.Option) {
	config, err := app.LoadConfig(&Config{}, nil)
	if err != nil {
		config.Logger.Error("failed to load configuration", slog.Any("error", err))
		os.Exit(1)
	}
	if config.GitRepo == "" {
		config.Logger.Error("missing required --git-repo (or git_repo in config)")
		os.Exit(1)
	}

	// Start with an empty FS; the poller swaps in the real templates once the
	// repo is available.
	config.TemplatesFS = afero.NewMemMapFs()
	config.Reload = gitReload(config)

	app.Serve(&config.Config, overrides...)
}

// gitReload polls url@ref and returns a channel that emits a WithTemplateFS
// option each time ref's commit changes, suitable for xtemplate.Config.Reload.
// It checks immediately, then every interval. Failures are logged and retried.
func gitReload(config *Config) <-chan []xtemplate.Option {
	ch := make(chan []xtemplate.Option)
	go func() {
		var last string
		var dirs []string // recent clones; kept alive while old instances drain
		check := func() {
			sha, err := lsRemote(config.Ctx, config.GitRepo, config.GitRef)
			if err != nil {
				config.Logger.Warn("ls-remote failed", slog.Any("error", err))
				return
			}
			if sha == last {
				return
			}
			newdir, err := clone(config.Ctx, config.GitRepo, config.GitRef)
			if err != nil {
				config.Logger.Warn("clone failed", slog.Any("error", err))
				return
			}
			config.Logger.Info("reloading from new commit", slog.String("commit", sha), slog.String("dir", newdir))
			select {
			case ch <- []xtemplate.Option{xtemplate.WithTemplateFS(subFS(newdir, config.TemplatesDir))}:
			case <-config.Ctx.Done():
				_ = os.RemoveAll(newdir)
				return
			}
			last = sha
			dirs = append(dirs, newdir)
			for len(dirs) > 2 {
				_ = os.RemoveAll(dirs[0])
				dirs = dirs[1:]
			}
		}

		check()
		ticker := time.NewTicker(config.GitInterval)
		defer ticker.Stop()
		for {
			select {
			case <-config.Ctx.Done():
				return
			case <-ticker.C:
				check()
			}
		}
	}()
	return ch
}

func subFS(dir, subdir string) afero.Fs {
	return afero.NewBasePathFs(afero.NewOsFs(), filepath.Join(dir, subdir))
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
	args = append(args, url, dir)
	if out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput(); err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("git clone: %w: %s", err, out)
	}
	return dir, nil
}

// lsRemote returns the commit SHA that ref points to on the remote without
// cloning. An empty ref resolves the remote's HEAD.
func lsRemote(ctx context.Context, url, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	out, err := exec.CommandContext(ctx, "git", "ls-remote", url, ref).Output()
	if err != nil {
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
	// A single-ref query returns one line with no trailing newline; Cut
	// reports ok=false and returns the whole string, which is the line we want.
	line, _, _ := strings.Cut(out, "\n")
	sha, _, ok := strings.Cut(line, "\t")
	if !ok || sha == "" {
		return "", fmt.Errorf("malformed ls-remote line: %q", line)
	}
	return sha, nil
}
