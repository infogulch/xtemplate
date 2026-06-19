package xtemplate

import (
	"context"
	"testing"
	"time"
)

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
}
