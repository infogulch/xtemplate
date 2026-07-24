package xtemplate_caddy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

// parse runs the Caddyfile handler directive parser over the given input and
// returns the resulting module, failing the test on parse error.
func parse(t *testing.T, input string) *XTemplateModule {
	t.Helper()
	h := httpcaddyfile.Helper{}.WithDispenser(caddyfile.NewTestDispenser(input))
	mh, err := parseCaddyfile(h)
	if err != nil {
		t.Fatalf("parseCaddyfile(%q) error: %v", input, err)
	}
	m, ok := mh.(*XTemplateModule)
	if !ok {
		t.Fatalf("parseCaddyfile returned %T, want *XTemplateModule", mh)
	}
	return m
}

// parseErr runs the parser and asserts that it returns an error.
func parseErr(t *testing.T, input string) error {
	t.Helper()
	h := httpcaddyfile.Helper{}.WithDispenser(caddyfile.NewTestDispenser(input))
	_, err := parseCaddyfile(h)
	if err == nil {
		t.Errorf("parseCaddyfile(%q) = nil error, want an error", input)
	}
	return err
}

func TestParseCaddyfile_Defaults(t *testing.T) {
	m := parse(t, `xtemplate`)

	// Default is os at Provision — no SourceRaw means default os.
	if len(m.SourceRaw) != 0 {
		t.Errorf("SourceRaw = %s, want empty (default os at Provision)", m.SourceRaw)
	}
	if m.TemplateExtension != ".html" {
		t.Errorf("TemplateExtension = %q, want %q", m.TemplateExtension, ".html")
	}
	if m.LDelim != "{{" || m.RDelim != "}}" {
		t.Errorf("delimiters = %q %q, want {{ }}", m.LDelim, m.RDelim)
	}
	if m.Minify == nil || !*m.Minify {
		t.Errorf("Minify = %v, want non-nil true by default", m.Minify)
	}
}

func TestParseCaddyfile_AllOptions(t *testing.T) {
	m := parse(t, `xtemplate {
		source os {
			path tpl
		}
		template_extension .gohtml
		delimiters [[ ]]
		minify false
		precompress gzip br
	}`)

	if len(m.SourceRaw) == 0 {
		t.Fatal("SourceRaw empty, want os source")
	}
	var src struct {
		Type string `json:"type"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(m.SourceRaw, &src); err != nil {
		t.Fatal(err)
	}
	if src.Type != "os" || src.Path != "tpl" {
		t.Errorf("source = %+v, want type=os path=tpl", src)
	}
	if m.TemplateExtension != ".gohtml" {
		t.Errorf("TemplateExtension = %q, want %q", m.TemplateExtension, ".gohtml")
	}
	if m.LDelim != "[[" || m.RDelim != "]]" {
		t.Errorf("delimiters = %q %q, want [[ ]]", m.LDelim, m.RDelim)
	}
	if m.Minify == nil || *m.Minify {
		t.Errorf("Minify = %v, want non-nil false", m.Minify)
	}
	if !equalStrings(m.Precompress, []string{"gzip", "br"}) {
		t.Errorf("Precompress = %v, want [gzip br]", m.Precompress)
	}
}

func TestParseCaddyfile_Precompress(t *testing.T) {
	m := parse(t, `xtemplate {
		precompress gzip
		precompress zstd br
	}`)
	want := []string{"gzip", "zstd", "br"}
	if !equalStrings(m.Precompress, want) {
		t.Errorf("Precompress = %v, want %v", m.Precompress, want)
	}
}

func TestParseCaddyfile_LegacyTemplatesDirRejected(t *testing.T) {
	err := parseErr(t, `xtemplate {
		templates_dir tpl
	}`)
	if err != nil && !strings.Contains(err.Error(), "no longer supported") {
		t.Errorf("error %q should mention no longer supported", err)
	}
}

func TestParseCaddyfile_LegacyWatchRejected(t *testing.T) {
	err := parseErr(t, `xtemplate {
		watch_template_path true
	}`)
	if err != nil && !strings.Contains(err.Error(), "no longer supported") {
		t.Errorf("error %q should mention no longer supported", err)
	}
}

func TestParseCaddyfile_TemplatesPathAliasRejected(t *testing.T) {
	err := parseErr(t, `xtemplate {
		templates_path legacy
	}`)
	if err != nil && !strings.Contains(err.Error(), "no longer supported") {
		t.Errorf("error %q should mention no longer supported", err)
	}
}

func TestParseCaddyfile_CrossOrigin(t *testing.T) {
	m := parse(t, `xtemplate {
		crossorigin {
			disabled true
			trusted_origins https://a.example https://b.example
			insecure_bypass_patterns /webhook /health
		}
	}`)

	if !m.CrossOrigin.Disabled {
		t.Error("CrossOrigin.Disabled = false, want true")
	}
	wantOrigins := []string{"https://a.example", "https://b.example"}
	if got := m.CrossOrigin.TrustedOrigins; !equalStrings(got, wantOrigins) {
		t.Errorf("TrustedOrigins = %v, want %v", got, wantOrigins)
	}
	wantPatterns := []string{"/webhook", "/health"}
	if got := m.CrossOrigin.InsecureBypassPatterns; !equalStrings(got, wantPatterns) {
		t.Errorf("InsecureBypassPatterns = %v, want %v", got, wantPatterns)
	}
}

func TestParseCaddyfile_Errors(t *testing.T) {
	cases := map[string]string{
		"unknown directive":        "xtemplate {\n\tbogus\n}",
		"non-bool minify":          "xtemplate {\n\tminify notabool\n}",
		"legacy watch":             "xtemplate {\n\twatch_template_path maybe\n}",
		"precompress no args":      "xtemplate {\n\tprecompress\n}",
		"precompress bad encoding": "xtemplate {\n\tprecompress lzma\n}",
		"unknown crossorigin opt":  "xtemplate {\n\tcrossorigin {\n\t\tbogus true\n\t}\n}",
		"non-bool crossorigin":     "xtemplate {\n\tcrossorigin {\n\t\tdisabled notabool\n\t}\n}",
		"missing trusted_origins":  "xtemplate {\n\tcrossorigin {\n\t\ttrusted_origins\n\t}\n}",
		"too few delimiters":       "xtemplate {\n\tdelimiters [[\n}",
		"legacy templates_dir":     "xtemplate {\n\ttemplates_dir\n}",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_ = parseErr(t, input)
		})
	}
}

// fakeProvider is a minimal CaddyfileBlockParser registered for tests.
type fakeProvider struct{}

func (fakeProvider) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "xtemplate.providers.fake",
		New: func() caddy.Module { return new(fakeProvider) },
	}
}

func (fakeProvider) ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error) {
	var dsn string
	for h.NextBlock(1) {
		switch h.Val() {
		case "dsn":
			if !h.AllArgs(&dsn) {
				return nil, h.ArgErr()
			}
		default:
			return nil, h.Errf("unknown fake option '%s'", h.Val())
		}
	}
	return json.Marshal(struct {
		DSN string `json:"dsn,omitempty"`
	}{dsn})
}

func init() {
	caddy.RegisterModule(fakeProvider{})
}

func TestParseCaddyfile_ProviderBlock(t *testing.T) {
	m := parse(t, "xtemplate {\n\tprovider fake DB {\n\t\tdsn postgres://localhost/mydb\n\t}\n}")

	if len(m.ProvidersRaw) != 1 {
		t.Fatalf("ProvidersRaw len = %d, want 1", len(m.ProvidersRaw))
	}
	var got struct {
		Type string `json:"type"`
		Name string `json:"name"`
		DSN  string `json:"dsn"`
	}
	if err := json.Unmarshal(m.ProvidersRaw[0], &got); err != nil {
		t.Fatalf("unmarshal ProvidersRaw[0]: %v", err)
	}
	if got.Type != "fake" {
		t.Errorf("type = %q, want %q", got.Type, "fake")
	}
	if got.Name != "DB" {
		t.Errorf("name = %q, want %q", got.Name, "DB")
	}
	if got.DSN != "postgres://localhost/mydb" {
		t.Errorf("dsn = %q, want %q", got.DSN, "postgres://localhost/mydb")
	}
}

func TestParseCaddyfile_ProviderErrors(t *testing.T) {
	cases := map[string]string{
		"missing type and field": "xtemplate {\n\tprovider\n}",
		"missing field name":     "xtemplate {\n\tprovider fake\n}",
		"unknown provider type":  "xtemplate {\n\tprovider nope Field {\n\t}\n}",
		"unknown block key":      "xtemplate {\n\tprovider fake Field {\n\t\tbogus val\n\t}\n}",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_ = parseErr(t, input)
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
