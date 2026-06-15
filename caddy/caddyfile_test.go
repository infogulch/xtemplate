package xtemplate_caddy

import (
	"testing"

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
func parseErr(t *testing.T, input string) {
	t.Helper()
	h := httpcaddyfile.Helper{}.WithDispenser(caddyfile.NewTestDispenser(input))
	if _, err := parseCaddyfile(h); err == nil {
		t.Errorf("parseCaddyfile(%q) = nil error, want an error", input)
	}
}

func TestParseCaddyfile_Defaults(t *testing.T) {
	m := parse(t, `xtemplate`)

	if !m.WatchTemplatePath {
		t.Error("WatchTemplatePath = false, want true by default")
	}
	if m.TemplatesDir != "templates" {
		t.Errorf("TemplatesDir = %q, want %q", m.TemplatesDir, "templates")
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
		templates_dir tpl
		template_extension .gohtml
		delimiters [[ ]]
		minify false
		watch_template_path false
	}`)

	if m.TemplatesDir != "tpl" {
		t.Errorf("TemplatesDir = %q, want %q", m.TemplatesDir, "tpl")
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
	if m.WatchTemplatePath {
		t.Error("WatchTemplatePath = true, want false")
	}
}

func TestParseCaddyfile_TemplatesPathAlias(t *testing.T) {
	m := parse(t, `xtemplate {
		templates_path legacy
	}`)
	if m.TemplatesDir != "legacy" {
		t.Errorf("TemplatesDir = %q, want %q (templates_path alias)", m.TemplatesDir, "legacy")
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
	// Inputs are newline-delimited like real Caddyfiles so closing braces are
	// not read as directive arguments.
	cases := map[string]string{
		"unknown directive":         "xtemplate {\n\tbogus\n}",
		"non-bool minify":           "xtemplate {\n\tminify notabool\n}",
		"non-bool watch":            "xtemplate {\n\twatch_template_path maybe\n}",
		"unknown crossorigin opt":   "xtemplate {\n\tcrossorigin {\n\t\tbogus true\n\t}\n}",
		"non-bool crossorigin":      "xtemplate {\n\tcrossorigin {\n\t\tdisabled notabool\n\t}\n}",
		"missing trusted_origins":   "xtemplate {\n\tcrossorigin {\n\t\ttrusted_origins\n\t}\n}",
		"too few delimiters":        "xtemplate {\n\tdelimiters [[\n}",
		"missing templates_dir arg": "xtemplate {\n\ttemplates_dir\n}",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			parseErr(t, input)
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
