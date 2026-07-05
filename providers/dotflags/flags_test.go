package dotflags_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/providers/dotflags"
)

func buildInstance(t *testing.T, files map[string]string, opts ...xtemplate.Option) *xtemplate.Instance {
	t.Helper()
	fs := afero.NewMemMapFs()
	for name, content := range files {
		if err := afero.WriteFile(fs, name, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %q to mem fs: %v", name, err)
		}
	}
	cfg := xtemplate.New()
	allOpts := append([]xtemplate.Option{xtemplate.WithTemplateFS(fs)}, opts...)
	inst, _, _, err := cfg.Instance(allOpts...)
	if err != nil {
		t.Fatalf("failed to build instance: %v", err)
	}
	return inst
}

func doRequest(inst *xtemplate.Instance, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, target, nil)
	inst.ServeHTTP(w, r)
	return w
}

func TestFlagsProvider(t *testing.T) {
	inst := buildInstance(t,
		map[string]string{
			"greet.html": `{{.Flags.Value "greeting"}}`,
		},
		dotflags.WithFlags("Flags", map[string]string{"greeting": "hi"}),
	)

	w := doRequest(inst, http.MethodGet, "/greet")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); !strings.Contains(body, "hi") {
		t.Errorf("body = %q, want it to contain %q", body, "hi")
	}
}
