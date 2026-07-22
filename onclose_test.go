package xtemplate

import (
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

func TestWithOnClose_RunsOnInstanceClose(t *testing.T) {
	var calls atomic.Int32
	fs := newMemFS(t, map[string]string{"index.html": "ok"})

	inst, _, _, err := New().Instance(
		WithTemplateFS(fs),
		WithOnClose(func() error {
			calls.Add(1)
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("Instance: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("OnClose ran before Close: %d", calls.Load())
	}
	if err := inst.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("OnClose calls = %d, want 1", calls.Load())
	}
	// Idempotent Close
	if err := inst.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("OnClose calls after second Close = %d, want 1", calls.Load())
	}
}

func TestWithOnClose_ReverseOrderAfterProviders(t *testing.T) {
	var mu sync.Mutex
	var seq []string
	add := func(s string) {
		mu.Lock()
		seq = append(seq, s)
		mu.Unlock()
	}

	fs := newMemFS(t, map[string]string{"index.html": "ok"})
	inst, _, _, err := New().Instance(
		WithTemplateFS(fs),
		WithProvider(&closeOrderProvider{name: "P", onClose: func() { add("provider") }}),
		WithOnClose(func() error { add("onClose1"); return nil }),
		WithOnClose(func() error { add("onClose2"); return nil }),
	)
	if err != nil {
		t.Fatalf("Instance: %v", err)
	}
	if err := inst.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Providers first, then OnClose in reverse registration order.
	want := []string{"provider", "onClose2", "onClose1"}
	mu.Lock()
	got := append([]string(nil), seq...)
	mu.Unlock()
	if len(got) != len(want) {
		t.Fatalf("seq = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("seq = %v, want %v", got, want)
		}
	}
}

func TestWithOnClose_BaseConfigRunsPerRetiredInstance(t *testing.T) {
	var calls atomic.Int32
	fs1 := newMemFS(t, map[string]string{"index.html": "v1"})
	fs2 := newMemFS(t, map[string]string{"index.html": "v2"})

	srv, err := New().Server(
		WithTemplateFS(fs1),
		WithOnClose(func() error {
			calls.Add(1)
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("Server: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("OnClose ran before any retire: %d", calls.Load())
	}

	if err := srv.Reload(WithTemplateFS(fs2)); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("after first Reload OnClose calls = %d, want 1 (old instance)", calls.Load())
	}

	srv.Stop()
	if calls.Load() != 2 {
		t.Fatalf("after Stop OnClose calls = %d, want 2", calls.Load())
	}
}

func TestWithOnClose_ResliceDoesNotTrampleBase(t *testing.T) {
	// Base config has one handler. Building instances that append more OnClose
	// via Options must not grow or corrupt the base slice.
	var baseCalls, extraCalls atomic.Int32
	baseFn := func() error { baseCalls.Add(1); return nil }
	extraFn := func() error { extraCalls.Add(1); return nil }

	cfg := New()
	if _, err := cfg.Options(
		WithTemplateFS(newMemFS(t, map[string]string{"index.html": "ok"})),
		WithOnClose(baseFn),
	); err != nil {
		t.Fatalf("Options: %v", err)
	}
	if len(cfg.onClose) != 1 {
		t.Fatalf("base onClose len = %d, want 1", len(cfg.onClose))
	}

	inst1, _, _, err := cfg.Instance(WithOnClose(extraFn))
	if err != nil {
		t.Fatalf("Instance1: %v", err)
	}
	if len(cfg.onClose) != 1 {
		t.Fatalf("base onClose len after Instance1 = %d, want 1 (reslice isolation)", len(cfg.onClose))
	}

	inst2, _, _, err := cfg.Instance(WithOnClose(extraFn))
	if err != nil {
		t.Fatalf("Instance2: %v", err)
	}
	if len(cfg.onClose) != 1 {
		t.Fatalf("base onClose len after Instance2 = %d, want 1", len(cfg.onClose))
	}

	_ = inst1.Close()
	_ = inst2.Close()
	if baseCalls.Load() != 2 {
		t.Fatalf("base OnClose calls = %d, want 2", baseCalls.Load())
	}
	if extraCalls.Load() != 2 {
		t.Fatalf("extra OnClose calls = %d, want 2", extraCalls.Load())
	}
}

func TestWithOnClose_BuildFailureRunsCallbacks(t *testing.T) {
	var calls atomic.Int32
	// Missing templates FS path that doesn't exist as dir with no default walk
	// — use invalid provider-free build: empty TemplatesDir on Os that fails scan
	// is hard. Force Options success then fail: unsupported precompress encoding.
	_, _, _, err := New().Instance(
		WithTemplateFS(newMemFS(t, map[string]string{"index.html": "ok"})),
		WithOnClose(func() error {
			calls.Add(1)
			return nil
		}),
		func(c *Config) error {
			c.Precompress = []string{"not-a-real-encoding"}
			return nil
		},
	)
	if err == nil {
		t.Fatal("Instance: want error for bad precompress")
	}
	if calls.Load() != 1 {
		t.Fatalf("OnClose on failed build calls = %d, want 1", calls.Load())
	}
}

func TestWithOnClose_ErrorJoined(t *testing.T) {
	fs := newMemFS(t, map[string]string{"index.html": "ok"})
	inst, _, _, err := New().Instance(
		WithTemplateFS(fs),
		WithOnClose(func() error { return errors.New("boom") }),
	)
	if err != nil {
		t.Fatalf("Instance: %v", err)
	}
	err = inst.Close()
	if err == nil || err.Error() == "" {
		t.Fatalf("Close error = %v, want boom", err)
	}
}

// closeOrderProvider is a minimal Closer for ordering tests.
type closeOrderProvider struct {
	name    string
	onClose func()
}

func (p *closeOrderProvider) FieldName() string { return p.name }
func (p *closeOrderProvider) Prototype() any    { return struct{}{} }
func (p *closeOrderProvider) Value(http.ResponseWriter, *http.Request) (any, error) {
	return struct{}{}, nil
}
func (p *closeOrderProvider) Close() error {
	if p.onClose != nil {
		p.onClose()
	}
	return nil
}
