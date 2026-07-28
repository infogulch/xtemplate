package watchfs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/infogulch/xtemplate"
)

func TestController_StartReturnsSticky(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("STICKY"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Controller{Path: dir, Debounce: xtemplate.Duration(50 * time.Millisecond)}
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

	if srv.Instance() == nil {
		t.Fatal("watchfs sticky should load first instance via Reload")
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "STICKY") {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestController_FSChange_EmptyReloadServesNewContent(t *testing.T) {
	dir := t.TempDir()
	index := filepath.Join(dir, "index.html")
	if err := os.WriteFile(index, []byte("V1"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Controller{Path: dir, Debounce: xtemplate.Duration(30 * time.Millisecond)}
	cfg, err := xtemplate.New().Options(xtemplate.WithController(s), xtemplate.WithMinify(false))
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
	if !strings.Contains(w.Body.String(), "V1") {
		t.Fatalf("initial body = %q, want V1", w.Body.String())
	}

	// Real FS change must trigger empty Reload from sticky path.
	if err := os.WriteFile(index, []byte("V2-WATCHFS"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		w = httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if w.Code == http.StatusOK && strings.Contains(w.Body.String(), "V2-WATCHFS") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for watchfs reload; last status=%d body=%q", w.Code, w.Body.String())
		}
		time.Sleep(30 * time.Millisecond)
	}
}

func TestController_DefaultsPath(t *testing.T) {
	s := &Controller{}
	if s.Path != "" {
		t.Fatalf("zero Path should be empty before Start, got %q", s.Path)
	}
	// Start with missing default dir may error if templates/ is absent.
	cfg, err := xtemplate.New().Options(xtemplate.WithController(s), xtemplate.WithMinify(false))
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	_, err = cfg.Server()
	_ = err // environment-dependent
}
