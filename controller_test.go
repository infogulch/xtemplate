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
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"
)

func TestResolveController_Unknown(t *testing.T) {
	_, err := ResolveController(json.RawMessage(`{"type":"watchfs"}`))
	if err == nil {
		t.Fatal("expected error for unregistered watchfs in core tests")
	}
	if !strings.Contains(err.Error(), "controllers/watchfs") {
		t.Errorf("error %q should hint at controllers/watchfs", err)
	}
}

func TestMaterializeController_FromRaw(t *testing.T) {
	cfg := &Config{ControllerRaw: json.RawMessage(`{"type":"os","path":"hello"}`)}
	name, err := cfg.MaterializeController("")
	if err != nil {
		t.Fatalf("MaterializeController: %v", err)
	}
	if name != "os" {
		t.Errorf("name = %q, want os", name)
	}
	osCtrl, ok := cfg.Controller.(*OsFsController)
	if !ok {
		t.Fatalf("Controller = %T, want *OsFsController", cfg.Controller)
	}
	if osCtrl.Path != "hello" {
		t.Errorf("Path = %q, want hello", osCtrl.Path)
	}
	if cfg.ControllerRaw != nil {
		t.Error("ControllerRaw should be cleared")
	}
}

func TestMaterializeController_PreferTypeMatchKeepsRaw(t *testing.T) {
	cfg := &Config{ControllerRaw: json.RawMessage(`{"type":"os","path":"from-json"}`)}
	name, err := cfg.MaterializeController("os")
	if err != nil {
		t.Fatalf("MaterializeController: %v", err)
	}
	if name != "os" {
		t.Errorf("name = %q, want os", name)
	}
	osCtrl, ok := cfg.Controller.(*OsFsController)
	if !ok || osCtrl.Path != "from-json" {
		t.Fatalf("matching prefer should keep raw path, got %T %#v", cfg.Controller, cfg.Controller)
	}
}

func TestMaterializeController_PreferTypeMismatchDiscardsRaw(t *testing.T) {
	// Non-empty preferType that differs from raw type discards Raw (CLI type wins).
	// Core only registers "os"; unregistered prefer still proves Raw is dropped first.
	cfg := &Config{ControllerRaw: json.RawMessage(`{"type":"os","path":"from-json"}`)}
	name, err := cfg.MaterializeController("not_registered")
	if err == nil {
		t.Fatal("expected error for unregistered preferType after discarding raw")
	}
	if name != "" {
		t.Errorf("name = %q on error, want empty", name)
	}
	if cfg.Controller != nil {
		t.Errorf("Controller should remain nil after failed NewController, got %T", cfg.Controller)
	}
	if cfg.ControllerRaw != nil {
		t.Error("ControllerRaw should be cleared on type mismatch before NewController")
	}
}

func TestMaterializeController_DefaultWhenEmpty(t *testing.T) {
	prev := DefaultControllerType
	DefaultControllerType = "os"
	t.Cleanup(func() { DefaultControllerType = prev })

	cfg := &Config{}
	name, err := cfg.MaterializeController("")
	if err != nil {
		t.Fatalf("MaterializeController: %v", err)
	}
	if name != "os" {
		t.Errorf("name = %q, want os", name)
	}
	if _, ok := cfg.Controller.(*OsFsController); !ok {
		t.Errorf("Controller = %T, want *OsFsController", cfg.Controller)
	}
}

func TestMaterializeController_AlreadySetClearsRaw(t *testing.T) {
	existing := &OsFsController{Path: "keep"}
	cfg := &Config{
		Controller:    existing,
		ControllerRaw: json.RawMessage(`{"type":"os","path":"ignored"}`),
	}
	name, err := cfg.MaterializeController("os")
	if err != nil {
		t.Fatalf("MaterializeController: %v", err)
	}
	if name != "" {
		t.Errorf("name = %q when already set, want empty (unknown)", name)
	}
	if cfg.Controller != existing {
		t.Error("existing Controller should be preserved")
	}
	if cfg.ControllerRaw != nil {
		t.Error("ControllerRaw should be cleared")
	}
	if existing.Path != "keep" {
		t.Errorf("Path = %q, want keep", existing.Path)
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

func TestServer_ControllerRaw_Materializes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("FROM-CONTROLLER-RAW"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]string{"type": "os", "path": dir})
	if err != nil {
		t.Fatal(err)
	}

	// Minify off so body is exact.
	cfg, _ := New().Options(WithMinify(false))
	cfg.ControllerRaw = raw

	srv, err := cfg.Server()
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "FROM-CONTROLLER-RAW") {
		t.Errorf("body = %q, want FROM-CONTROLLER-RAW", w.Body.String())
	}
}

func TestInstance_RequiresTemplateFS(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("INSTANCE-DIR"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := false

	// No TemplateFS.
	cfg := New()
	cfg.Minify = &f
	_, err := cfg.Instance()
	if err == nil {
		t.Fatal("Instance without TemplateFS should error")
	}
	if !strings.Contains(err.Error(), "template FS") && !strings.Contains(err.Error(), "WithTemplateFS") {
		t.Errorf("error %q should mention template FS requirement", err)
	}

	// Controller is rejected even when TemplateFS is also set.
	cfg, err = New().Options(
		WithTemplateDir(dir),
		WithController(&OsFsController{Path: dir}),
		func(c *Config) error { c.Minify = &f; return nil },
	)
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	_, err = cfg.Instance()
	if err == nil {
		t.Fatal("Instance with Controller should error")
	}
	if !strings.Contains(err.Error(), "Controller") {
		t.Errorf("error %q should mention Controller", err)
	}

	// ControllerRaw is rejected.
	cfg, err = New().Options(WithTemplateDir(dir), func(c *Config) error { c.Minify = &f; return nil })
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	cfg.ControllerRaw = json.RawMessage(`{"type":"os","path":"x"}`)
	_, err = cfg.Instance()
	if err == nil {
		t.Fatal("Instance with ControllerRaw should error")
	}
	if !strings.Contains(err.Error(), "Controller") {
		t.Errorf("error %q should mention Controller", err)
	}

	// WithTemplateDir alone works.
	cfg, err = New().Options(
		WithTemplateDir(dir),
		func(c *Config) error { c.Minify = &f; return nil },
	)
	if err != nil {
		t.Fatalf("Options(WithTemplateDir): %v", err)
	}
	inst, err := cfg.Instance()
	if err != nil {
		t.Fatalf("Instance(WithTemplateDir): %v", err)
	}
	defer func() { _ = inst.Close() }()

	w := httptest.NewRecorder()
	inst.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "INSTANCE-DIR") {
		t.Errorf("body = %q, want INSTANCE-DIR", w.Body.String())
	}
}

func TestServer_Sticky_EmptyReload_StillServes(t *testing.T) {
	srv := buildServer(t, map[string]string{"index.html": "BASE-V1"})
	defer srv.Stop()

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "BASE-V1") {
		t.Fatalf("first load: status=%d body=%q", w.Code, w.Body.String())
	}

	if err := srv.Reload(); err != nil {
		t.Fatalf("empty Reload: %v", err)
	}
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(w.Body.String(), "BASE-V1") {
		t.Errorf("after empty Reload body = %q, want BASE-V1", w.Body.String())
	}
}

func TestServer_NonStickyReload_RestoresBaseFS(t *testing.T) {
	srv := buildServer(t, map[string]string{"index.html": "BASE-V1"})
	defer srv.Stop()

	if err := srv.Reload(WithTemplateFS(newMemFS(t, map[string]string{"index.html": "RELOAD-V2"}))); err != nil {
		t.Fatalf("Reload V2: %v", err)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(w.Body.String(), "RELOAD-V2") {
		t.Fatalf("after V2 body = %q", w.Body.String())
	}

	// Empty reload rebuilds from sticky base, not last reload opts.
	if err := srv.Reload(); err != nil {
		t.Fatalf("empty Reload: %v", err)
	}
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(w.Body.String(), "BASE-V1") {
		t.Errorf("after empty Reload body = %q, want BASE-V1", w.Body.String())
	}
}

func TestServer_Deferred_NilInstance_503ThenContent(t *testing.T) {
	// Deferred controller has no sticky FS — cannot use buildServer (it always sets TemplateFS).
	cfg, err := New().Options(
		WithController(&testController{}),
	)
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	srv, err := cfg.Server()
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	if srv.Instance() != nil {
		t.Fatal("deferred controller: want nil instance until Reload")
	}
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut} {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest(method, "/anything", nil))
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status = %d, want 503", method, w.Code)
		}
	}

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

func TestServer_ReloadWithoutFS_ErrorsRetainsPrevious(t *testing.T) {
	cfg, err := New().Options(
		WithController(&testController{}),
	)
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	srv, err := cfg.Server()
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	if err := srv.Reload(WithTemplateFS(newMemFS(t, map[string]string{"index.html": "LIVE"}))); err != nil {
		t.Fatalf("content Reload: %v", err)
	}

	if err := srv.Reload(); err == nil {
		t.Fatal("Reload() without sticky/FS should fail")
	}
	if err := srv.Reload(WithOnClose(func() error { return nil })); err == nil {
		t.Fatal("Reload(WithOnClose) without FS should fail")
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

func TestServer_RejectedReload_RunsOnClose(t *testing.T) {
	closed := make(chan struct{}, 1)
	cfg, err := New().Options(
		WithController(&testController{}),
	)
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	srv, err := cfg.Server()
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	if err := srv.Reload(WithTemplateFS(newMemFS(t, map[string]string{"index.html": "LIVE"}))); err != nil {
		t.Fatalf("content Reload: %v", err)
	}

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
	if !strings.Contains(err.Error(), "template root") && !strings.Contains(err.Error(), "WithTemplateFS") {
		t.Errorf("error %q should mention missing template root", err)
	}
	select {
	case <-closed:
	default:
		t.Error("OnClose was not called for rejected empty reload")
	}
}

func TestServer_FailedReload_KeepsPrevious_RunsOnClose(t *testing.T) {
	var closed atomic.Int32
	srv := buildServer(t, map[string]string{"index.html": "KEEP"})
	defer srv.Stop()

	// Bad build: unsupported precompress encoding after FS set.
	err := srv.Reload(
		WithTemplateFS(newMemFS(t, map[string]string{"index.html": "BAD"})),
		WithOnClose(func() error {
			closed.Add(1)
			return nil
		}),
		func(c *Config) error {
			c.Precompress = []string{"not-a-real-encoding"}
			return nil
		},
	)
	if err == nil {
		t.Fatal("want build failure")
	}
	if closed.Load() != 1 {
		t.Fatalf("OnClose calls = %d, want 1", closed.Load())
	}

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(w.Body.String(), "KEEP") {
		t.Errorf("body = %q, want KEEP (previous instance retained)", w.Body.String())
	}
}

func TestWithController_RejectedOnReload(t *testing.T) {
	srv := buildServer(t, map[string]string{"index.html": "ok"})
	defer srv.Stop()
	err := srv.Reload(WithController(&OsFsController{Path: "other"}))
	if err == nil {
		t.Fatal("Reload(WithController) should error")
	}
	if !strings.Contains(err.Error(), "Controller") {
		t.Errorf("error %q should mention Controller", err)
	}
}

func TestRegisterProvider_IsPublicName(t *testing.T) {
	// Compile-time / runtime: RegisterProvider is the public registration API.
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
	for i := 0; i < 2; i++ {
		srv := buildServer(t, map[string]string{"index.html": "SERVER-OS-BODY-MARKER"})
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

// testController: nil sticky; callers supply content via Reload.
type testController struct {
	options []Option
	err     error
}

var _ ServerController = (*testController)(nil)

func (s *testController) Init(_ context.Context, _ *slog.Logger) ([]Option, error) {
	return s.options, s.err
}

func (s *testController) Start(_ *Server) error { return nil }

func TestServer_ControllerStartMayReloadSync(t *testing.T) {
	fs := newMemFS(t, map[string]string{"index.html": "SYNC-RELOAD"})
	ctrl := &syncReloadController{fs: fs}
	cfg, err := New().Options(WithController(ctrl), WithMinify(false))
	if err != nil {
		t.Fatalf("Options: %v", err)
	}

	srv, err := cfg.Server()
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "SYNC-RELOAD") {
		t.Errorf("body = %q, want SYNC-RELOAD", w.Body.String())
	}
}

type syncReloadController struct {
	fs afero.Fs
}

var _ ServerController = (*syncReloadController)(nil)

func (c *syncReloadController) Init(_ context.Context, _ *slog.Logger) ([]Option, error) {
	return nil, nil
}

func (c *syncReloadController) Start(server *Server) error {
	return server.Reload(WithTemplateFS(c.fs))
}
