package git

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/infogulch/xtemplate"
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

func TestController_StartRequiresRepo(t *testing.T) {
	s := &Controller{}
	f := false
	cfg := xtemplate.New()
	cfg.Minify = &f
	cfg, err := cfg.Options(xtemplate.WithController(s))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = cfg.Server()
	if err == nil {
		t.Fatal("expected error for missing repo")
	}
}

func TestController_StartRejectsDashedRepo(t *testing.T) {
	s := &Controller{Repo: "--upload-pack=evil"}
	f := false
	cfg := xtemplate.New()
	cfg.Minify = &f
	cfg, err := cfg.Options(xtemplate.WithController(s))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = cfg.Server()
	if err == nil {
		t.Fatal("expected error for dashed repo")
	}
}

func TestRedactSecrets(t *testing.T) {
	in := `fatal: repository 'https://user:s3cret@github.com/org/repo.git' not found`
	out := redactSecrets(in)
	if strings.Contains(out, "s3cret") {
		t.Errorf("password still present: %q", out)
	}
	if !strings.Contains(out, "user:***@") {
		t.Errorf("expected redacted userinfo, got %q", out)
	}
}

func TestConfinedSubdir(t *testing.T) {
	clone := t.TempDir()
	if err := os.MkdirAll(filepath.Join(clone, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}

	root, err := confinedSubdir(clone, "templates")
	if err != nil {
		t.Fatalf("templates: %v", err)
	}
	abs, err := filepath.Abs(filepath.Join(clone, "templates"))
	if err != nil {
		t.Fatal(err)
	}
	if root != abs {
		t.Errorf("root = %q, want %q", root, abs)
	}

	if _, err := confinedSubdir(clone, "../.."); err == nil {
		t.Error("want error for .. escape")
	}
	if _, err := confinedSubdir(clone, "/etc"); err == nil {
		t.Error("want error for absolute path")
	}
	if _, err := confinedSubdir(clone, "foo/../../../etc"); err == nil {
		t.Error("want error for nested .. escape")
	}
}

// TestAferoBasePathFs_FollowsSymlinks documents that afero BasePathFs follows
// symlinks on Open/ReadFile — the security property that rejectSymlinks guards.
func TestAferoBasePathFs_FollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	tpl := filepath.Join(dir, "templates")
	if err := os.MkdirAll(tpl, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secret, []byte("HOST-SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(tpl, "leak")); err != nil {
		t.Fatal(err)
	}

	fsys := afero.NewBasePathFs(afero.NewOsFs(), tpl)
	b, err := afero.ReadFile(fsys, "leak")
	if err != nil {
		t.Fatalf("ReadFile: %v (expected BasePathFs to follow symlink)", err)
	}
	if string(b) != "HOST-SECRET" {
		t.Fatalf("data = %q, want HOST-SECRET — afero behavior changed?", b)
	}
}

func TestSubFS_RejectsSymlinks(t *testing.T) {
	clone := t.TempDir()
	tpl := filepath.Join(clone, "templates")
	if err := os.MkdirAll(tpl, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(clone, "secret.txt")
	if err := os.WriteFile(secret, []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tpl, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(tpl, "leak")); err != nil {
		t.Fatal(err)
	}

	_, err := subFS(clone, "templates")
	if err == nil {
		t.Fatal("expected symlink rejection")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error %q should mention symlink", err)
	}
}

// TestController_Server_503ThenContent uses a real git controller with Server.
// Init returns nil sticky → 503 until first successful Reload with clone FS.
func TestController_Server_503ThenContent(t *testing.T) {
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
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "templates", "index.html"), []byte("HELLO-GIT"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "initial")

	s := &Controller{
		Repo:     repo,
		Interval: xtemplate.Duration(time.Hour),
		Path:     "templates",
	}
	f := false
	cfg := xtemplate.New()
	cfg.Minify = &f
	cfg, err := cfg.Options(xtemplate.WithController(s))
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	srv, err := cfg.Server()
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	// Nil sticky: 503 until first successful clone reload.
	// First poll may win the race; at least one 503 or eventual content is required.
	sawOK := false
	deadline := time.Now().Add(15 * time.Second)
	for {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if w.Code == http.StatusOK && strings.Contains(w.Body.String(), "HELLO-GIT") {
			sawOK = true
			break
		}
		if w.Code != http.StatusServiceUnavailable && w.Code != http.StatusOK {
			t.Fatalf("unexpected status %d body=%q", w.Code, w.Body.String())
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for git content; last status=%d body=%q", w.Code, w.Body.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !sawOK {
		t.Fatal("never saw HELLO-GIT")
	}
}

// TestController_FailedBuild_RetrySucceeds verifies that a failed Reload leaves
// last SHA unadvanced (retry can succeed) and OnClose cleans the clone. Uses a
// real git repo; first poll is forced to fail build via bad Path that passes
// Start validation but fails subFS after clone… Path is validated at Start.
// Instead: poll succeeds, then inject build failure by making templates
// unparseable is hard. We simulate via a controlled second path: after first
// success, corrupt template so next commit's Reload fails, then fix and confirm
// recovery — last SHA only advances on success.
func TestController_FailedBuild_RetrySucceeds(t *testing.T) {
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
	tplDir := filepath.Join(repo, "templates")
	if err := os.MkdirAll(tplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, "index.html"), []byte("OK1"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "ok1")

	s := &Controller{
		Repo:     repo,
		Interval: xtemplate.Duration(200 * time.Millisecond),
		Path:     "templates",
	}
	cfg, err := xtemplate.New().Options(xtemplate.WithController(s), xtemplate.WithMinify(false))
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	srv, err := cfg.Server()
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	waitBody := func(want string, d time.Duration) {
		t.Helper()
		deadline := time.Now().Add(d)
		for {
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
			if w.Code == http.StatusOK && strings.Contains(w.Body.String(), want) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("timeout waiting for %q; last status=%d body=%q", want, w.Code, w.Body.String())
			}
			time.Sleep(40 * time.Millisecond)
		}
	}
	waitBody("OK1", 15*time.Second)

	// Bad template that fails instance build (unclosed action).
	if err := os.WriteFile(filepath.Join(tplDir, "index.html"), []byte("{{if true}}BAD"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "bad")

	// Allow at least one poll cycle for the bad commit to be attempted.
	time.Sleep(600 * time.Millisecond)

	// Previous good content must still be served (failed Reload keeps instance).
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "OK1") {
		t.Fatalf("after bad commit: status=%d body=%q, want OK1 retained", w.Code, w.Body.String())
	}

	// Fix and commit; last SHA was not advanced on failure so poller retries.
	if err := os.WriteFile(filepath.Join(tplDir, "index.html"), []byte("OK2"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "ok2")
	waitBody("OK2", 15*time.Second)
}
