package xtemplate

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
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

func TestWithOnClose_ResliceCapGreaterThanLen(t *testing.T) {
	// slices.Clone alone is not enough if Options ran first against a shared
	// backing array with spare capacity. Grow base to cap>len, then build an
	// instance that appends more OnClose — base must stay untrampled.
	var baseN, instN, lateN atomic.Int32
	cfg := New()
	if _, err := cfg.Options(
		WithTemplateFS(newMemFS(t, map[string]string{"index.html": "ok"})),
		WithOnClose(func() error { baseN.Add(1); return nil }),
		WithOnClose(func() error { baseN.Add(1); return nil }),
		WithOnClose(func() error { baseN.Add(1); return nil }),
	); err != nil {
		t.Fatalf("Options: %v", err)
	}
	if len(cfg.onClose) != 3 {
		t.Fatalf("base onClose len = %d, want 3", len(cfg.onClose))
	}
	if cap(cfg.onClose) <= len(cfg.onClose) {
		// Force spare capacity: copy into a larger slice so append can write
		// past len without reallocating — the trampling case clone-before-Options prevents.
		grown := make([]func() error, len(cfg.onClose), len(cfg.onClose)+4)
		copy(grown, cfg.onClose)
		cfg.onClose = grown
	}
	if cap(cfg.onClose) <= len(cfg.onClose) {
		t.Fatalf("need cap>len for trampling regression; cap=%d len=%d", cap(cfg.onClose), len(cfg.onClose))
	}

	inst, _, _, err := cfg.Instance(WithOnClose(func() error { instN.Add(1); return nil }))
	if err != nil {
		t.Fatalf("Instance: %v", err)
	}
	if len(cfg.onClose) != 3 {
		t.Fatalf("base onClose len after Instance = %d, want 3", len(cfg.onClose))
	}

	// Appending to base must not pick up the instance-only handler from a shared array.
	if _, err := cfg.Options(WithOnClose(func() error { lateN.Add(1); return nil })); err != nil {
		t.Fatalf("late Options: %v", err)
	}
	if len(cfg.onClose) != 4 {
		t.Fatalf("base onClose len after late append = %d, want 4", len(cfg.onClose))
	}

	_ = inst.Close()
	if baseN.Load() != 3 {
		t.Fatalf("base handlers during inst.Close = %d, want 3", baseN.Load())
	}
	if instN.Load() != 1 {
		t.Fatalf("instance handler calls = %d, want 1", instN.Load())
	}
	if lateN.Load() != 0 {
		t.Fatalf("late base handler ran during inst.Close: %d", lateN.Load())
	}

	// New instance from base should get the 4 base handlers only.
	baseN.Store(0)
	lateN.Store(0)
	inst2, _, _, err := cfg.Instance()
	if err != nil {
		t.Fatalf("Instance2: %v", err)
	}
	_ = inst2.Close()
	if baseN.Load() != 3 {
		t.Fatalf("Instance2 base handlers = %d, want 3", baseN.Load())
	}
	if lateN.Load() != 1 {
		t.Fatalf("Instance2 late handler = %d, want 1", lateN.Load())
	}
	if instN.Load() != 1 {
		t.Fatalf("instance-only handler leaked into base: calls=%d", instN.Load())
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

func TestWithOnClose_OptionsFailureRunsCallbacks(t *testing.T) {
	// WithOnClose applied before a later Option error must still run on the
	// fail path (closeOnce points at the same Instance config field Options mutates).
	var calls atomic.Int32
	_, _, _, err := New().Instance(
		WithOnClose(func() error {
			calls.Add(1)
			return nil
		}),
		func(*Config) error { return errors.New("option-fail") },
	)
	if err == nil {
		t.Fatal("Instance: want option-fail")
	}
	if !strings.Contains(err.Error(), "option-fail") {
		t.Fatalf("err = %v, want option-fail", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("OnClose on Options failure calls = %d, want 1", calls.Load())
	}
}

func TestWithOnClose_BuildFailureJoinsCloseErrors(t *testing.T) {
	_, _, _, err := New().Instance(
		WithTemplateFS(newMemFS(t, map[string]string{"index.html": "ok"})),
		WithOnClose(func() error { return errors.New("close-boom") }),
		func(c *Config) error {
			c.Precompress = []string{"not-a-real-encoding"}
			return nil
		},
	)
	if err == nil {
		t.Fatal("Instance: want error")
	}
	if !strings.Contains(err.Error(), "not-a-real-encoding") && !strings.Contains(err.Error(), "unsupported encoding") {
		t.Fatalf("err = %v, want build failure about encoding", err)
	}
	if !strings.Contains(err.Error(), "close-boom") {
		t.Fatalf("err = %v, want joined close-boom from OnClose", err)
	}
}

func TestWithOnClose_InitFailureClosesOpenedProviders(t *testing.T) {
	var mu sync.Mutex
	var seq []string
	add := func(s string) {
		mu.Lock()
		seq = append(seq, s)
		mu.Unlock()
	}

	_, _, _, err := New().Instance(
		WithTemplateFS(newMemFS(t, map[string]string{"index.html": "ok"})),
		WithProvider(&initCloseProvider{
			name:    "OK",
			onClose: func() { add("provider-ok") },
		}),
		WithProvider(&initCloseProvider{
			name:    "Fail",
			initErr: errors.New("init-fail"),
		}),
		WithOnClose(func() error { add("onClose"); return nil }),
	)
	if err == nil {
		t.Fatal("Instance: want init failure")
	}
	if !strings.Contains(err.Error(), "init-fail") {
		t.Fatalf("err = %v, want init-fail", err)
	}
	// Provider that successfully Init'd must Close; user OnClose must run.
	// Fail provider never Init'd successfully so it is not on the close list.
	want := []string{"provider-ok", "onClose"}
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

func TestClose_NilReceiver(t *testing.T) {
	var inst *Instance
	if err := inst.Close(); err != nil {
		t.Fatalf("nil Close: %v, want nil", err)
	}
}

func TestClose_SecondCloseReturnsMemoizedError(t *testing.T) {
	// OnceValue memoizes the first Close error; second Close must not re-run
	// callbacks but does return the same error.
	var calls atomic.Int32
	inst, _, _, err := New().Instance(
		WithTemplateFS(newMemFS(t, map[string]string{"index.html": "ok"})),
		WithOnClose(func() error {
			calls.Add(1)
			return errors.New("boom")
		}),
	)
	if err != nil {
		t.Fatalf("Instance: %v", err)
	}
	err1 := inst.Close()
	if err1 == nil || !strings.Contains(err1.Error(), "boom") {
		t.Fatalf("first Close: %v, want boom", err1)
	}
	err2 := inst.Close()
	if err2 == nil || !strings.Contains(err2.Error(), "boom") {
		t.Fatalf("second Close: %v, want memoized boom", err2)
	}
	if calls.Load() != 1 {
		t.Fatalf("OnClose calls = %d, want 1 (one-shot)", calls.Load())
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

// initCloseProvider is Initializer+Closer for mid-list Init failure tests.
type initCloseProvider struct {
	name    string
	initErr error
	onClose func()
}

func (p *initCloseProvider) FieldName() string { return p.name }
func (p *initCloseProvider) Prototype() any    { return struct{}{} }
func (p *initCloseProvider) Value(http.ResponseWriter, *http.Request) (any, error) {
	return struct{}{}, nil
}
func (p *initCloseProvider) Init(context.Context) error { return p.initErr }
func (p *initCloseProvider) Close() error {
	if p.onClose != nil {
		p.onClose()
	}
	return nil
}

func TestClose_WarnsWhenInflight(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	inst, _, _, err := New().Instance(
		WithTemplateFS(newMemFS(t, map[string]string{"index.html": "ok"})),
		func(c *Config) error {
			c.Logger = log
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Instance: %v", err)
	}

	// Simulate a request still in ServeHTTP when Close runs (grace expired).
	inst.inflight.Add(1)

	if err := inst.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "closing instance with unresolved requests") {
		t.Fatalf("log = %q, want warn about unresolved requests", out)
	}
	if !strings.Contains(out, "inflight=1") {
		t.Fatalf("log = %q, want inflight=1", out)
	}

	// Second Close must not re-warn (handlers may still be draining).
	buf.Reset()
	if err := inst.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("second Close logged %q, want no re-warn", buf.String())
	}
}

func TestClose_NoWarnWhenIdle(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	inst, _, _, err := New().Instance(
		WithTemplateFS(newMemFS(t, map[string]string{"index.html": "ok"})),
		func(c *Config) error {
			c.Logger = log
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Instance: %v", err)
	}
	if err := inst.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("idle Close logged %q, want silence", buf.String())
	}
}
