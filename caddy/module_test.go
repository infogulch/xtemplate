package xtemplate_caddy

import (
	"encoding/json"
	"html/template"
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

// TestModuleUnmarshalJSON guards that the full xtemplate.Config surface remains
// reachable through the embedded Config when configuring the module via Caddy's
// JSON config. If a Config field's json tag changes or embedding breaks, this
// test catches it.
func TestModuleUnmarshalJSON(t *testing.T) {
	const cfg = `{
		"minify": true,
		"templates_dir": "templates",
		"watch_template_path": true,
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
	if m.TemplatesDir != "templates" {
		t.Errorf("TemplatesDir = %q, want %q", m.TemplatesDir, "templates")
	}
	if !m.WatchTemplatePath {
		t.Error("WatchTemplatePath = false, want true")
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
