package dotbus_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/providers/dotbus"
)

func buildInstance(t *testing.T, files map[string]string, opts ...xtemplate.Option) *xtemplate.Instance {
	t.Helper()
	fs := afero.NewMemMapFs()
	for name, content := range files {
		if err := afero.WriteFile(fs, name, []byte(content), 0644); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	cfg, err := xtemplate.New().Options(append([]xtemplate.Option{xtemplate.WithTemplateFS(fs)}, opts...)...)
	if err != nil {
		t.Fatalf("failed to apply options: %v", err)
	}
	inst, err := cfg.Instance()
	if err != nil {
		t.Fatalf("failed to build instance: %v", err)
	}
	return inst
}

// sseRecorder is an http.ResponseWriter that signals when SSE data is written.
// Safe for concurrent Write + wait (unlike reading httptest.ResponseRecorder.Body).
type sseRecorder struct {
	header http.Header
	code   int
	mu     sync.Mutex
	body   strings.Builder
	saw    chan struct{}
	once   sync.Once
}

func newSSERecorder() *sseRecorder {
	return &sseRecorder{
		header: make(http.Header),
		saw:    make(chan struct{}),
	}
}

func (r *sseRecorder) Header() http.Header        { return r.header }
func (r *sseRecorder) WriteHeader(statusCode int) { r.code = statusCode }
func (r *sseRecorder) Flush()                     {}
func (r *sseRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	r.body.Write(p)
	r.mu.Unlock()
	if bytes.Contains(p, []byte("data:")) {
		r.once.Do(func() { close(r.saw) })
	}
	return len(p), nil
}

func (r *sseRecorder) BodyString() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.String()
}

func TestDotBus_PublishSubscribeTemplate(t *testing.T) {
	inst := buildInstance(t,
		map[string]string{
			"pub.html": `{{.Bus.Publish "messages" "hi"}}ok`,
			"index.html": `{{define "SSE /events"}}` +
				`{{range .Bus.Subscribe "messages"}}{{$.Flush.SendSSE "" .}}{{end}}{{end}}`,
		},
		dotbus.WithBus("Bus", 8),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := newSSERecorder()
	r := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	r.Header.Set("Accept", "text/event-stream")

	done := make(chan struct{})
	go func() {
		inst.ServeHTTP(w, r)
		close(done)
	}()

	// Wait for the SSE handler to subscribe, then publish.
	time.Sleep(50 * time.Millisecond)
	pw := httptest.NewRecorder()
	inst.ServeHTTP(pw, httptest.NewRequest(http.MethodGet, "/pub", nil))
	if pw.Code != http.StatusOK {
		cancel()
		<-done
		t.Fatalf("publish status = %d body %q", pw.Code, pw.Body.String())
	}

	select {
	case <-w.saw:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatalf("sse body = %q, want data: hi", w.BodyString())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not return after cancel")
	}
	if !strings.Contains(w.BodyString(), "data: hi") {
		t.Fatalf("sse body = %q, want data: hi", w.BodyString())
	}
}

func TestDotBusConfig_Init(t *testing.T) {
	cfg := &dotbus.DotBusConfig{Name: "Bus"}
	ctx, cancel := context.WithCancel(context.Background())
	if err := cfg.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	v, err := cfg.Value(nil, httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	dot := v.(*dotbus.DotBus)
	ch, err := dot.Subscribe("t")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := dot.Publish("t", "m"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case got := <-ch:
		if got != "m" {
			t.Fatalf("got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
	cancel() // Kick via Init's ctx watcher closes subscriber channels
	select {
	case _, ok := <-ch:
		if ok {
			for ok {
				_, ok = <-ch
			}
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for channel close after Kick")
	}
	// Kick leaves the bus open for Publish.
	if err := dot.Publish("t", "after-kick"); err != nil {
		t.Fatalf("Publish after Kick: %v", err)
	}
	if err := cfg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := dot.Publish("t", "x"); err == nil {
		t.Fatal("Publish after Close should fail")
	}
}

func TestDotBusConfig_Validation(t *testing.T) {
	if err := (&dotbus.DotBusConfig{}).Init(context.Background()); err == nil {
		t.Fatal("expected error for empty name")
	}
	if err := (&dotbus.DotBusConfig{Name: "B", Buffer: -1}).Init(context.Background()); err == nil {
		t.Fatal("expected error for negative buffer")
	}
}

func TestDotBus_WithBusOption(t *testing.T) {
	inst := buildInstance(t,
		map[string]string{
			"index.html": `{{.Bus.Publish "x" "y"}}ok`,
		},
		dotbus.WithBus("Bus", 0),
	)
	w := httptest.NewRecorder()
	inst.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}

// TestDotBus_ReloadIsolatesInstances ensures sticky WithBus factories produce
// a new bus per Reload, so retiring the previous instance does not Close the
// live bus (the sticky-provider Init/Close bug).
func TestDotBus_ReloadIsolatesInstances(t *testing.T) {
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, "pub.html", []byte(`{{.Bus.Publish "t" "hi"}}ok`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := xtemplate.New().Options(
		xtemplate.WithTemplateFS(fs),
		dotbus.WithBus("Bus", 8),
	)
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	srv, err := cfg.Server()
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	publishOK := func(label string) {
		t.Helper()
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/pub", nil))
		if w.Code != http.StatusOK || w.Body.String() != "ok" {
			t.Fatalf("%s: status=%d body=%q (bus may have been closed by prior instance)", label, w.Code, w.Body.String())
		}
	}

	publishOK("initial")
	if err := srv.Reload(); err != nil {
		t.Fatalf("Reload 1: %v", err)
	}
	publishOK("after reload 1")
	if err := srv.Reload(); err != nil {
		t.Fatalf("Reload 2: %v", err)
	}
	publishOK("after reload 2")
}

func TestDotBus_RequestCancelUnsubscribes(t *testing.T) {
	cfg := &dotbus.DotBusConfig{Name: "Bus", Buffer: 4}
	if err := cfg.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	reqCtx, cancel := context.WithCancel(context.Background())
	v, err := cfg.Value(nil, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(reqCtx))
	if err != nil {
		t.Fatal(err)
	}
	dot := v.(*dotbus.DotBus)
	ch, err := dot.Subscribe("t")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel after request cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for unsubscribe")
	}
}
