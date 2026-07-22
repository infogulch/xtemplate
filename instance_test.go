package xtemplate

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
)

// newMemFS returns an in-memory afero.Fs populated with the given files. Each
// key is a path and each value is the file's contents.
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

// buildInstance builds an Instance from an in-memory template fs and any extra
// options, failing the test if construction fails.
func buildInstance(t *testing.T, files map[string]string, opts ...Option) *Instance {
	t.Helper()
	fs := newMemFS(t, files)
	cfg := New()
	allOpts := append([]Option{WithTemplateFS(fs)}, opts...)
	inst, _, _, err := cfg.Instance(allOpts...)
	if err != nil {
		t.Fatalf("failed to build instance: %v", err)
	}
	return inst
}

// doRequest issues an in-process request against the instance and returns the
// recorder.
func doRequest(inst *Instance, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, target, nil)
	inst.ServeHTTP(w, r)
	return w
}

func TestServeHTTP_IndexRoute(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"index.html": "INDEX-MARKER",
	})

	w := doRequest(inst, http.MethodGet, "/")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); !strings.Contains(body, "INDEX-MARKER") {
		t.Errorf("body = %q, want it to contain %q", body, "INDEX-MARKER")
	}
}

func TestServeHTTP_DirIndexRoute(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"dir/index.html": "DIR-INDEX-MARKER",
	})

	// The directory's index is served at its canonical trailing-slash URL.
	w := doRequest(inst, http.MethodGet, "/dir/")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /dir/ status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); !strings.Contains(body, "DIR-INDEX-MARKER") {
		t.Errorf("body = %q, want it to contain %q", body, "DIR-INDEX-MARKER")
	}

	// ServeMux auto-redirects the slashless form to the canonical URL.
	w = doRequest(inst, http.MethodGet, "/dir")
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /dir status = %d, want %d", w.Code, http.StatusMovedPermanently)
	}
	if loc := w.Header().Get("Location"); loc != "/dir/" {
		t.Errorf("GET /dir Location = %q, want %q", loc, "/dir/")
	}

	// The index route is an exact match, NOT a subtree: unmatched sub-paths 404.
	w = doRequest(inst, http.MethodGet, "/dir/nonexistent")
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /dir/nonexistent status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestServeHTTP_NamedFileRoute(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"hello.html": "HELLO-MARKER",
	})

	w := doRequest(inst, http.MethodGet, "/hello")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); !strings.Contains(body, "HELLO-MARKER") {
		t.Errorf("body = %q, want it to contain %q", body, "HELLO-MARKER")
	}
}

func TestServeHTTP_HiddenFileNotRouted(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		".secret.html": "SECRET-MARKER",
	})

	w := doRequest(inst, http.MethodGet, "/.secret")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestServeHTTP_StaticFileAndHash(t *testing.T) {
	const css = "body { color: red; }\n"
	inst := buildInstance(t, map[string]string{
		"style.css": css,
		// template that exposes the precomputed hash of the static file
		"hash.html": `{{.X.StaticFileHash "/style.css"}}`,
	})

	// Static asset is served with the right content type and an Etag.
	w := doRequest(inst, http.MethodGet, "/style.css")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != css {
		t.Errorf("body = %q, want %q", got, css)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/css; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/css; charset=utf-8")
	}
	etag := w.Header().Get("Etag")
	if etag == "" {
		t.Fatalf("Etag header is empty, want a hash")
	}
	if !strings.HasPrefix(etag, `"sha384-`) {
		t.Errorf("Etag = %q, want it to start with %q", etag, `"sha384-`)
	}

	// The hash exposed to templates matches the Etag value (sans quotes).
	w2 := doRequest(inst, http.MethodGet, "/hash")
	if w2.Code != http.StatusOK {
		t.Fatalf("hash template status = %d, want %d", w2.Code, http.StatusOK)
	}
	wantHash := strings.Trim(etag, `"`)
	if got := strings.TrimSpace(w2.Body.String()); got != wantHash {
		t.Errorf("StaticFileHash = %q, want %q", got, wantHash)
	}
}

func TestServer_EmptyFSWithHandler(t *testing.T) {
	// xtemplate must build from an empty FS (zero routes) so a server can start
	// before its templates are available, and custom handlers must still serve.
	hit := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusNoContent)
	})

	cfg := New()
	server, err := cfg.Server(
		WithTemplateFS(afero.NewMemMapFs()),
		WithHandler("POST /hook", handler),
	)
	if err != nil {
		t.Fatalf("failed to build server from empty FS: %v", err)
	}
	defer server.Stop()

	w := httptest.NewRecorder()
	server.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/hook", nil))
	if !hit {
		t.Error("custom handler was not invoked")
	}
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestInstance_SkipsHiddenDirs(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"index.html":         "VISIBLE",
		".git/secret.html":   "SECRET",
		".git/config.txt":    "junk",
		".hidden/static.txt": "hidden static",
		"normaldir/a.html":   "hello",
	})

	// .html is stripped from route paths and index maps to the dir root.
	for _, target := range []string{"/", "/normaldir/a"} {
		if w := doRequest(inst, http.MethodGet, target); w.Code != http.StatusOK {
			t.Errorf("visible route %s status = %d, want %d", target, w.Code, http.StatusOK)
		}
	}
	for _, target := range []string{"/.git/secret.html", "/.git/config.txt", "/.hidden/static.txt"} {
		if w := doRequest(inst, http.MethodGet, target); w.Code != http.StatusNotFound {
			t.Errorf("hidden route %s status = %d, want %d", target, w.Code, http.StatusNotFound)
		}
	}
}

func TestServer_ReloadChannel(t *testing.T) {
	fs1 := newMemFS(t, map[string]string{"index.html": "V1"})
	fs2 := newMemFS(t, map[string]string{"index.html": "V2"})

	reload := make(chan []Option)
	cfg := New()
	cfg.Reload = reload
	server, err := cfg.Server(WithTemplateFS(fs1))
	if err != nil {
		t.Fatalf("failed to build server: %v", err)
	}
	defer server.Stop()

	get := func() string {
		w := httptest.NewRecorder()
		server.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		return w.Body.String()
	}

	if body := get(); !strings.Contains(body, "V1") {
		t.Fatalf("before reload body = %q, want it to contain V1", body)
	}

	// Send options through the channel to swap the templates FS, then wait for
	// the consumer goroutine to apply the reload by polling for the new content.
	reload <- []Option{WithTemplateFS(fs2)}

	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(get(), "V2") {
		if time.Now().After(deadline) {
			t.Fatalf("after reload body = %q, want it to contain V2", get())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestServer_Lifecycle(t *testing.T) {
	fs := newMemFS(t, map[string]string{
		"index.html": "INDEX-MARKER",
	})
	cfg := New()
	server, err := cfg.Server(WithTemplateFS(fs))
	if err != nil {
		t.Fatalf("failed to build server: %v", err)
	}

	inst := server.Instance()
	if inst == nil {
		t.Fatalf("server.Instance() = nil after build, want non-nil")
	}

	// Requests routed through the handler reach the current instance.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	server.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// Reload succeeds and yields a non-nil (possibly new) instance.
	if err := server.Reload(); err != nil {
		t.Fatalf("server.Reload() failed: %v", err)
	}
	if server.Instance() == nil {
		t.Fatalf("server.Instance() = nil after Reload, want non-nil")
	}

	// After Stop the instance pointer is cleared (new requests get 503).
	server.Stop()
	if server.Instance() != nil {
		t.Errorf("server.Instance() = %v after Stop, want nil", server.Instance())
	}
}

func TestServer_HandlerAfterStop(t *testing.T) {
	fs := newMemFS(t, map[string]string{
		"index.html": "INDEX-MARKER",
	})
	cfg := New()
	server, err := cfg.Server(WithTemplateFS(fs))
	if err != nil {
		t.Fatalf("failed to build server: %v", err)
	}

	server.Stop()

	// Routing a request through the handler after Stop must not panic and must
	// respond with 503 Service Unavailable instead of dereferencing a nil
	// instance.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("handler panicked after Stop: %v", p)
		}
	}()
	server.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
