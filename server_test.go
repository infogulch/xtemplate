package xtemplate

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	os.Exit(m.Run())
}

// TestServe_ReturnsOnCtxCancel verifies that cancelling Config.Ctx shuts down
// the running server so Serve returns instead of blocking forever.
func TestServe_ReturnsOnCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := New()
	cfg.Ctx = ctx
	cfg.TemplatesFS = newMemFS(t, map[string]string{"index.html": "ok"})

	srv, err := cfg.Server()
	if err != nil {
		t.Fatalf("Server: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- srv.Serve("127.0.0.1:0") }()

	// Give Serve a moment to start listening, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after Ctx cancel")
	}

	if srv.Instance() != nil {
		t.Error("Instance() non-nil after Serve returned on cancel, want nil")
	}
}

func TestShutdown_IdempotentAnd503(t *testing.T) {
	cfg := New()
	srv, err := cfg.Server(WithTemplateFS(newMemFS(t, map[string]string{"index.html": "ok"})))
	if err != nil {
		t.Fatalf("Server: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
	srv.Stop() // also idempotent with Shutdown

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestStop_CancelsInstanceCtxWithoutParentCancel(t *testing.T) {
	// Embedder leaves Config.Ctx live; Stop must still cancel instance work.
	cfg := New()
	srv, err := cfg.Server(WithTemplateFS(newMemFS(t, map[string]string{
		"index.html": "ok",
		// Initial Flush signals the test that WaitForServerStop is about to block.
		"sse.html": `{{define "SSE /wait"}}{{.Flush.Flush}}{{.Flush.WaitForServerStop}}data: bye{{printf "\n\n"}}{{end}}`,
	})))
	if err != nil {
		t.Fatalf("Server: %v", err)
	}

	// Parent Config.Ctx must still be live after Stop for this test's point.
	select {
	case <-cfg.Ctx.Done():
		t.Fatal("Config.Ctx should still be live before Stop")
	default:
	}

	body := runSSEUntilStop(t, srv.Instance(), "/wait", func() {
		select {
		case <-cfg.Ctx.Done():
			t.Error("Config.Ctx was cancelled; Stop should not require parent cancel")
		default:
		}
		srv.Stop()
	})
	if !strings.Contains(body, "data: bye") {
		t.Fatalf("body = %q, want final data: bye after WaitForServerStop", body)
	}

	select {
	case <-cfg.Ctx.Done():
		t.Error("Config.Ctx should remain live after Stop")
	default:
	}
}

func TestReload_RetiresOldInstanceEarlyWhenIdle(t *testing.T) {
	cfg := New()
	srv, err := cfg.Server(WithTemplateFS(newMemFS(t, map[string]string{"index.html": "V1"})))
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	start := time.Now()
	if err := srv.Reload(WithTemplateFS(newMemFS(t, map[string]string{"index.html": "V2"}))); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	elapsed := time.Since(start)
	// Grace is 5s; idle retire must not burn most of it.
	if elapsed > 500*time.Millisecond {
		t.Fatalf("idle Reload retire took %v, want << defaultGrace", elapsed)
	}

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(w.Body.String(), "V2") {
		t.Fatalf("body = %q, want V2", w.Body.String())
	}
}

func TestReload_AllowsInFlightSSEFinalWrite(t *testing.T) {
	fs := func(marker string) map[string]string {
		return map[string]string{
			"index.html": marker,
			"sse.html":   `{{define "SSE /wait"}}{{.Flush.Flush}}{{.Flush.WaitForServerStop}}data: done{{printf "\n\n"}}{{end}}`,
		}
	}
	srv, err := New().Server(WithTemplateFS(newMemFS(t, fs("before"))))
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	defer srv.Stop()

	body := runSSEUntilStop(t, srv.Instance(), "/wait", func() {
		if err := srv.Reload(WithTemplateFS(newMemFS(t, fs("after")))); err != nil {
			t.Errorf("Reload: %v", err)
		}
	})
	if !strings.Contains(body, "data: done") {
		t.Fatalf("old SSE body = %q, want final event after instance cancel", body)
	}

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(w.Body.String(), "after") {
		t.Fatalf("new instance body = %q, want after", w.Body.String())
	}
}

func TestShutdown_DrainsInFlightHandler(t *testing.T) {
	srv, err := New().Server(WithTemplateFS(newMemFS(t, map[string]string{
		"index.html": "ok",
		"sse.html":   `{{define "SSE /wait"}}{{.Flush.Flush}}{{.Flush.WaitForServerStop}}data: drained{{printf "\n\n"}}{{end}}`,
	})))
	if err != nil {
		t.Fatalf("Server: %v", err)
	}

	body := runSSEUntilStop(t, srv.Instance(), "/wait", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	if !strings.Contains(body, "data: drained") {
		t.Fatalf("body = %q, want drained final event", body)
	}

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestReload_AfterStopFails(t *testing.T) {
	srv, err := New().Server(WithTemplateFS(newMemFS(t, map[string]string{"index.html": "ok"})))
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	srv.Stop()
	if err := srv.Reload(); err == nil {
		t.Fatal("Reload after Stop: want error")
	}
}

// runSSEUntilStop starts an SSE request on inst that blocks in WaitForServerStop
// (template must Flush first so we can observe start), then runs stopFn and
// returns the response body.
func runSSEUntilStop(t *testing.T, inst *Instance, path string, stopFn func()) string {
	t.Helper()
	if inst == nil {
		t.Fatal("instance is nil")
	}

	started := make(chan struct{})
	done := make(chan string, 1)
	go func() {
		w := &blockingSSERecorder{started: started}
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Accept", "text/event-stream")
		inst.ServeHTTP(w, r)
		done <- w.Body.String()
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not start")
	}

	// Let the template reach WaitForServerStop after the initial Flush.
	time.Sleep(20 * time.Millisecond)
	stopFn()

	select {
	case body := <-done:
		return body
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not finish after stop")
		return ""
	}
}

// blockingSSERecorder implements http.ResponseWriter and http.Flusher and
// signals when the first write or flush occurs.
type blockingSSERecorder struct {
	HeaderMap http.Header
	Body      strings.Builder
	Code      int
	started   chan struct{}
	once      sync.Once
}

func (w *blockingSSERecorder) Header() http.Header {
	if w.HeaderMap == nil {
		w.HeaderMap = make(http.Header)
	}
	return w.HeaderMap
}

func (w *blockingSSERecorder) Write(b []byte) (int, error) {
	w.signalStarted()
	if w.Code == 0 {
		w.Code = http.StatusOK
	}
	return w.Body.Write(b)
}

func (w *blockingSSERecorder) WriteHeader(statusCode int) {
	w.Code = statusCode
}

func (w *blockingSSERecorder) Flush() {
	w.signalStarted()
}

func (w *blockingSSERecorder) signalStarted() {
	w.once.Do(func() {
		if w.started != nil {
			close(w.started)
		}
	})
}

var (
	_ http.ResponseWriter = (*blockingSSERecorder)(nil)
	_ http.Flusher        = (*blockingSSERecorder)(nil)
)
