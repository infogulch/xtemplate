package dotfs_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/providers/dotfs"
)

func newMemFS(t *testing.T, files map[string]string) afero.Fs {
	t.Helper()
	fs := afero.NewMemMapFs()
	for name, content := range files {
		if err := afero.WriteFile(fs, name, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %q to mem fs: %v", name, err)
		}
	}
	return fs
}

func buildInstance(t *testing.T, files map[string]string, opts ...xtemplate.Option) *xtemplate.Instance {
	t.Helper()
	fs := newMemFS(t, files)
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

func TestDirProvider(t *testing.T) {
	dataFS := newMemFS(t, map[string]string{
		"message.txt": "FILE-CONTENT",
	})
	inst := buildInstance(t,
		map[string]string{
			"read.html": `{{.Files.Read "message.txt"}}`,
		},
		dotfs.WithFs("Files", dataFS),
	)

	w := doRequest(inst, http.MethodGet, "/read")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); !strings.Contains(body, "FILE-CONTENT") {
		t.Errorf("body = %q, want it to contain %q", body, "FILE-CONTENT")
	}
}
