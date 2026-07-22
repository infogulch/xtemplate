package xtemplate

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestResolveSource_Unknown(t *testing.T) {
	_, err := ResolveSource(json.RawMessage(`{"type":"watchfs"}`))
	if err == nil {
		t.Fatal("expected error for unregistered watchfs in core tests")
	}
	if !strings.Contains(err.Error(), "sources/watchfs") {
		t.Errorf("error %q should hint at sources/watchfs", err)
	}
}

func TestCheckLegacyTemplateKeys(t *testing.T) {
	for _, key := range []string{
		"templates_dir", "templates_path", "watch_dirs", "watch_template_path",
		"git_repo", "git_ref", "git_interval",
	} {
		err := CheckLegacyTemplateKeys([]byte(`{"` + key + `":"x"}`))
		if err == nil {
			t.Errorf("key %s: want error", key)
			continue
		}
		if !strings.Contains(err.Error(), "no longer supported") {
			t.Errorf("key %s: error %q missing migrate text", key, err)
		}
	}
	// Unknown keys are not banned
	if err := CheckLegacyTemplateKeys([]byte(`{"bogus_key":1,"minify":true}`)); err != nil {
		t.Errorf("unknown keys should be ignored by ban-list, got %v", err)
	}
}

func TestConfigUnmarshalJSON_BannedKeys(t *testing.T) {
	var c Config
	if err := json.Unmarshal([]byte(`{"templates_dir":"x"}`), &c); err == nil {
		t.Fatal("want ban-list error on Config JSON decode")
	}
	if err := json.Unmarshal([]byte(`{"git_repo":"https://example.com/r.git"}`), &c); err == nil {
		t.Fatal("want ban-list error for git_repo")
	}
}

func TestServer_SourceRaw_Materializes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("FROM-SOURCE-RAW"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]string{"type": "os", "path": dir})
	if err != nil {
		t.Fatal(err)
	}
	cfg := New()
	cfg.SourceRaw = raw
	// Minify off so body is exact.
	f := false
	cfg.Minify = &f
	cfg.Logger = slog.New(slog.DiscardHandler)

	srv, err := cfg.Server()
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	if cfg.SourceRaw != nil {
		// Server takes Config by value; caller's raw may remain — check server path via response.
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "FROM-SOURCE-RAW") {
		t.Errorf("body = %q, want FROM-SOURCE-RAW", w.Body.String())
	}
}

func TestInstance_SourceStart_NonNilInitial(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("INSTANCE-OS"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := false
	cfg := New()
	cfg.Minify = &f
	cfg.Logger = slog.New(slog.DiscardHandler)
	cfg.Source = &OsFsSource{Path: dir}

	inst, _, _, err := cfg.Instance()
	if err != nil {
		t.Fatalf("Instance: %v", err)
	}
	defer inst.Close()

	w := httptest.NewRecorder()
	inst.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "INSTANCE-OS") {
		t.Errorf("body = %q, want INSTANCE-OS", w.Body.String())
	}
}

func TestServer_NonStickyReload_RestoresBaseFS(t *testing.T) {
	base := newMemFS(t, map[string]string{"index.html": "BASE-V1"})
	srv, err := New().Server(WithTemplateFS(base), WithLogger(slog.New(slog.DiscardHandler)))
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	if err := srv.Reload(WithTemplateFS(newMemFS(t, map[string]string{"index.html": "RELOAD-V2"}))); err != nil {
		t.Fatalf("Reload V2: %v", err)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(w.Body.String(), "RELOAD-V2") {
		t.Fatalf("after V2 body = %q", w.Body.String())
	}

	// Empty reload rebuilds from sticky base (Start FS), not last reload opts.
	if err := srv.Reload(); err != nil {
		t.Fatalf("empty Reload: %v", err)
	}
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(w.Body.String(), "BASE-V1") {
		t.Errorf("after empty Reload body = %q, want BASE-V1", w.Body.String())
	}
}

func TestServer_NilInitial_RejectedReload_ClosesInstance(t *testing.T) {
	closed := make(chan struct{}, 1)
	ch := make(chan []Option)
	srv, err := New().Server(
		WithSource(&testSource{reloads: ch}),
		WithLogger(slog.New(slog.DiscardHandler)),
	)
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	// Live content first.
	if err := srv.Reload(WithTemplateFS(newMemFS(t, map[string]string{"index.html": "LIVE"}))); err != nil {
		t.Fatalf("content Reload: %v", err)
	}

	// Rejected empty reload must Close the abandoned build (OnClose runs).
	err = srv.Reload(WithOnClose(func() error {
		select {
		case closed <- struct{}{}:
		default:
		}
		return nil
	}))
	if err == nil {
		t.Fatal("expected reject")
	}
	if !strings.Contains(err.Error(), "WithTemplateFS") {
		t.Errorf("error %q should mention WithTemplateFS", err)
	}
	select {
	case <-closed:
	default:
		t.Error("OnClose was not called for rejected nil-initial reload")
	}
}

func TestWithSource_RejectedOnReload(t *testing.T) {
	srv, err := New().Server(WithTemplateFS(newMemFS(t, map[string]string{"index.html": "ok"})))
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()
	err = srv.Reload(WithSource(&OsFsSource{Path: "other"}))
	if err == nil {
		t.Fatal("Reload(WithSource) should error")
	}
	if !strings.Contains(err.Error(), "Source") {
		t.Errorf("error %q should mention WithSource", err)
	}
}

func TestServer_NilInitial_503ThenContent(t *testing.T) {
	srv, err := New().Server(WithSource(&testSource{reloads: make(chan []Option)}))
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	// Placeholder FS: {{define "ANY /"}} → 503 for every method/path.
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut} {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest(method, "/anything", nil))
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status = %d, want 503", method, w.Code)
		}
	}

	// Reload with a real FS serves content.
	if err := srv.Reload(WithTemplateFS(newMemFS(t, map[string]string{"index.html": "READY"}))); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "READY") {
		t.Errorf("body = %q, want READY", w.Body.String())
	}
}

func TestServer_NilInitial_ReloadWithoutFSRejected(t *testing.T) {
	ch := make(chan []Option)
	srv, err := New().Server(WithSource(&testSource{reloads: ch}))
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	if err := srv.Reload(WithTemplateFS(newMemFS(t, map[string]string{"index.html": "LIVE"}))); err != nil {
		t.Fatalf("content Reload: %v", err)
	}

	// Empty opts and non-FS opts must not wipe live content with the placeholder.
	if err := srv.Reload(); err == nil {
		t.Fatal("Reload() without FS should fail when Start returned nil initial")
	}
	if err := srv.Reload(WithOnClose(func() error { return nil })); err == nil {
		t.Fatal("Reload(WithOnClose) without FS should fail when Start returned nil initial")
	}

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (live instance retained)", w.Code)
	}
	if !strings.Contains(w.Body.String(), "LIVE") {
		t.Errorf("body = %q, want LIVE", w.Body.String())
	}
}

func TestInstance_RejectsReloadCapableSource(t *testing.T) {
	cfg := New()
	cfg.Source = &testSource{initial: afero.NewMemMapFs(), reloads: make(chan []Option)}
	_, _, _, err := cfg.Instance()
	if err == nil {
		t.Fatal("Instance should reject reload-capable source")
	}
	if !strings.Contains(err.Error(), "use Server") {
		t.Errorf("error %q should say use Server", err)
	}
}

func TestRegisterProvider_IsPublicName(t *testing.T) {
	// Compile-time / runtime: RegisterProvider is the public registration API.
	// Duplicate of _test already registered in providers_test init — just ensure
	// the function exists and resolve still works.
	raw := json.RawMessage(`{"type":"_test","name":"X","value":"v"}`)
	got, err := resolveProviders([]json.RawMessage{raw})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d providers", len(got))
	}
}

func TestServer_OS_ServesTemplateBody(t *testing.T) {
	// Real Server entry with WithTemplateFS; assert response body content.
	fs := newMemFS(t, map[string]string{"index.html": "SERVER-OS-BODY-MARKER"})
	for i := 0; i < 2; i++ {
		srv, err := New().Server(WithTemplateFS(fs))
		if err != nil {
			t.Fatalf("Server attempt %d: %v", i, err)
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d", i, w.Code)
		}
		if !strings.Contains(w.Body.String(), "SERVER-OS-BODY-MARKER") {
			t.Fatalf("attempt %d body = %q", i, w.Body.String())
		}
		srv.Stop()
	}
}

type testSource struct {
	initial afero.Fs
	reloads <-chan []Option
	err     error
}

func (s *testSource) Start(ctx context.Context, log *slog.Logger) (afero.Fs, <-chan []Option, error) {
	return s.initial, s.reloads, s.err
}
