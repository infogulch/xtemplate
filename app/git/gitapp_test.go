package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/app"
	"github.com/spf13/afero"
)

func TestParseLsRemote(t *testing.T) {
	sha, err := parseLsRemote("9d291430abc\trefs/heads/main\n0000\trefs/heads/other\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sha != "9d291430abc" {
		t.Errorf("sha = %q, want %q", sha, "9d291430abc")
	}

	if _, err := parseLsRemote("   \n"); err == nil {
		t.Error("expected error for empty output, got nil")
	}
	if _, err := parseLsRemote("no-tab-here"); err == nil {
		t.Error("expected error for malformed line, got nil")
	}
}

// TestGitReload_PollsLocalRepo exercises the full poller against a real local
// git repo. It is a regression test for a nil Config.Ctx: gitReload shells out
// with exec.CommandContext(config.Ctx, ...), which panics on a nil context, so
// this would crash on startup if LoadConfig failed to initialize Ctx.
func TestGitReload_PollsLocalRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}

	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "test")
	if err := os.MkdirAll(filepath.Join(repo, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "templates", "index.html"), []byte("HELLO"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "initial")

	config, err := app.LoadConfig(&Config{}, []string{"--git-repo", repo})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if config.Ctx == nil {
		t.Fatal("config.Ctx is nil after LoadConfig; gitReload would panic in exec.CommandContext")
	}

	// Use a testing context so the poller goroutine stops at test end.
	config.Ctx = t.Context()
	config.GitInterval = time.Hour // rely on the immediate first check

	ch := gitReload(config)

	var opts []xtemplate.Option
	select {
	case opts = <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for gitReload to emit a reload option")
	}

	// Apply the emitted options and confirm the templates FS serves the repo
	// contents from under the templates dir.
	var c xtemplate.Config
	for _, o := range opts {
		if err := o(&c); err != nil {
			t.Fatalf("applying reload option: %v", err)
		}
	}
	if c.TemplatesFS == nil {
		t.Fatal("reload option did not set TemplatesFS")
	}
	got, err := afero.ReadFile(c.TemplatesFS, "index.html")
	if err != nil {
		t.Fatalf("reading index.html from cloned FS: %v", err)
	}
	if string(got) != "HELLO" {
		t.Errorf("index.html = %q, want %q", got, "HELLO")
	}
}
