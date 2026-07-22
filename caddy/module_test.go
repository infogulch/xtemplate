package xtemplate_caddy

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caddyserver/caddy/v2"
)

func TestModuleRegistered(t *testing.T) {
	mi, err := caddy.GetModule("http.handlers.xtemplate")
	if err != nil {
		t.Fatalf("module http.handlers.xtemplate not registered: %v", err)
	}
	if _, ok := mi.New().(*XTemplateModule); !ok {
		t.Fatalf("module New() returned %T, want *XTemplateModule", mi.New())
	}
}

// TestModuleUnmarshalJSON guards that the Config surface remains reachable
// through the embedded Config when configuring via Caddy JSON.
func TestModuleUnmarshalJSON(t *testing.T) {
	const cfg = `{
		"minify": true,
		"source": {"type": "os", "path": "templates"},
		"funcs_modules": ["testfuncs"],
		"crossorigin": {
			"disabled": false,
			"trusted_origins": ["https://a.example"],
			"insecure_bypass_patterns": ["/hook"]
		},
		"providers": [
			{"type": "sql", "name": "DB", "driver": "sqlite3", "connstr": "file:./test.sqlite"},
			{"type": "fs", "name": "FS", "path": "data"},
			{"type": "flags", "name": "Flags", "values": {"a": "1"}}
		]
	}`

	var m XTemplateModule
	if err := json.Unmarshal([]byte(cfg), &m); err != nil {
		t.Fatalf("failed to unmarshal module config: %v", err)
	}

	if m.Minify == nil || !*m.Minify {
		t.Errorf("Minify = %v, want non-nil true", m.Minify)
	}
	if len(m.SourceRaw) == 0 {
		t.Error("SourceRaw empty, want source object")
	}
	if got := m.FuncsModules; !equalStrings(got, []string{"testfuncs"}) {
		t.Errorf("FuncsModules = %v, want [testfuncs]", got)
	}
	if got := m.CrossOrigin.TrustedOrigins; !equalStrings(got, []string{"https://a.example"}) {
		t.Errorf("TrustedOrigins = %v, want [https://a.example]", got)
	}
	if got := m.CrossOrigin.InsecureBypassPatterns; !equalStrings(got, []string{"/hook"}) {
		t.Errorf("InsecureBypassPatterns = %v, want [/hook]", got)
	}
	if len(m.ProvidersRaw) != 3 {
		t.Errorf("ProvidersRaw len = %d, want 3", len(m.ProvidersRaw))
	}
}

func TestModuleUnmarshalJSON_BannedKeys(t *testing.T) {
	for _, key := range []string{
		"templates_dir", "templates_path", "watch_dirs", "watch_template_path",
		"git_repo", "git_ref", "git_interval",
	} {
		raw := `{"` + key + `": true}`
		var m XTemplateModule
		err := json.Unmarshal([]byte(raw), &m)
		if err == nil {
			t.Errorf("key %s: want ban-list error", key)
			continue
		}
		if !strings.Contains(err.Error(), "no longer supported") {
			t.Errorf("key %s: error %q", key, err)
		}
	}
}

// TestServer_SourceRaw_ViaModuleConfig ensures Caddy-style SourceRaw is honored
// when Config.Server runs (not only when app.LoadConfig materializes).
func TestServer_SourceRaw_ViaModuleConfig(t *testing.T) {
	dir := t.TempDir()
	marker := "CADDY-SOURCE-RAW"
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]string{"type": "os", "path": dir})
	if err != nil {
		t.Fatal(err)
	}
	f := false
	m := &XTemplateModule{}
	m.SourceRaw = raw
	m.Minify = &f
	m.SetDefaults()
	m.Logger = slog.New(slog.DiscardHandler)

	srv, err := m.Server()
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), marker) {
		t.Errorf("body = %q, want %s", w.Body.String(), marker)
	}
}

// testFuncsModule is a minimal xtemplate.funcs module used to exercise the
// FuncsModules resolution path.
type testFuncsModule struct{}

func (testFuncsModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "xtemplate.funcs.testfuncs",
		New: func() caddy.Module { return testFuncsModule{} },
	}
}

func (testFuncsModule) Funcs() template.FuncMap {
	return template.FuncMap{"testfunc": func() string { return "hello" }}
}

func init() {
	caddy.RegisterModule(testFuncsModule{})
}

func TestResolveFuncsModules(t *testing.T) {
	fps, err := resolveFuncsModules([]string{"testfuncs"})
	if err != nil {
		t.Fatalf("resolveFuncsModules([testfuncs]) = %v, want nil", err)
	}
	if len(fps) != 1 {
		t.Fatalf("got %d providers, want 1", len(fps))
	}

	if _, err := resolveFuncsModules([]string{"does_not_exist"}); err == nil {
		t.Error("resolveFuncsModules([does_not_exist]) = nil error, want an error")
	}
}

func TestProvisionFuncsModules(t *testing.T) {
	fps, err := resolveFuncsModules([]string{"testfuncs"})
	if err != nil {
		t.Fatalf("resolveFuncsModules error: %v", err)
	}
	funcMaps, err := provisionFuncsModules(caddy.Context{}, fps)
	if err != nil {
		t.Fatalf("provisionFuncsModules error: %v", err)
	}
	if len(funcMaps) != 1 {
		t.Fatalf("got %d func maps, want 1", len(funcMaps))
	}
	if _, ok := funcMaps[0]["testfunc"]; !ok {
		t.Errorf("func map = %v, want it to contain 'testfunc'", funcMaps[0])
	}
}
