package xtemplate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Layer 1: SendSSE event framing.
//
// SendSSE is almost pure formatting logic, so it is exercised directly against
// an httptest.ResponseRecorder (which satisfies the unexported flusher
// interface) without standing up a server. These cases pin the exact wire bytes
// of the event stream, the multi-line data behavior, and the newline-injection
// guard that strips anything after the first line of event/id/retry.
// ---------------------------------------------------------------------------

func TestSendSSE(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"event only", []string{"hello"}, "event: hello\n\n\n", false},
		{"event and data", []string{"update", "payload"}, "event: update\ndata: payload\n\n\n", false},
		{"data only with empty event", []string{"", "line"}, "data: line\n\n\n", false},
		{"multiline data emits one data line each", []string{"", "a\nb\nc"}, "data: a\ndata: b\ndata: c\n\n\n", false},
		{"all four fields in order", []string{"ev", "d", "42", "3000"}, "event: ev\ndata: d\nid: 42\nretry: 3000\n\n\n", false},

		// Newline-injection guard: event/id/retry keep only their first line, so a
		// crafted value cannot smuggle in an extra SSE field.
		{"event newline injection stripped", []string{"ev\ninjected: x"}, "event: ev\n\n\n", false},
		{"id newline injection stripped", []string{"", "", "id1\ninjected: x"}, "id: id1\n\n\n", false},
		{"retry newline injection stripped", []string{"", "", "", "5000\ninjected: x"}, "retry: 5000\n\n\n", false},

		// All-empty args write nothing at all (the `written` guard), so no trailing
		// blank line and no flush.
		{"all empty writes nothing", []string{""}, "", false},
		{"four empty args write nothing", []string{"", "", "", ""}, "", false},

		// Arg-count validation.
		{"zero args errors", []string{}, "", true},
		{"five args errors", []string{"a", "b", "c", "d", "e"}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			f := &DotFlush{flusher: rec}

			err := f.SendSSE(tt.args...)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("SendSSE(%q) error = nil, want an error", tt.args)
				}
			} else if err != nil {
				t.Fatalf("SendSSE(%q) error = %v, want nil", tt.args, err)
			}

			if got := rec.Body.String(); got != tt.want {
				t.Errorf("body = %q, want %q", got, tt.want)
			}

			// SendSSE flushes exactly when it writes something.
			wantFlushed := tt.want != ""
			if rec.Flushed != wantFlushed {
				t.Errorf("flushed = %v, want %v", rec.Flushed, wantFlushed)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Layer 2: lifecycle helpers and context cancellation.
//
// Repeat, Sleep, and WaitForServerStop all key off the request and server
// contexts. These are constructed directly with controllable contexts (the test
// lives in package xtemplate, so the unexported fields are reachable) because
// cancellation behavior can't be exercised over the wire.
// ---------------------------------------------------------------------------

// newDotFlush builds a DotFlush wired to a throwaway recorder and the given
// request/server contexts.
func newDotFlush(reqCtx, srvCtx context.Context) *DotFlush {
	return &DotFlush{
		flusher:    httptest.NewRecorder(),
		requestCtx: reqCtx,
		serverCtx:  srvCtx,
	}
}

// drainUntilClosed reads from c until it is closed, failing the test if that
// does not happen promptly. Used to assert Repeat shuts its channel down.
func drainUntilClosed(t *testing.T, c <-chan int) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		for range c {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Repeat channel did not close after cancellation")
	}
}

func TestRepeat_YieldsBoundedSequence(t *testing.T) {
	f := newDotFlush(context.Background(), context.Background())

	var got []int
	for i := range f.Repeat(3) {
		got = append(got, i)
	}

	want := []int{0, 1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Repeat(3) yielded %v, want %v", got, want)
	}
}

func TestRepeat_StopsOnRequestCancel(t *testing.T) {
	reqCtx, cancel := context.WithCancel(context.Background())
	f := newDotFlush(reqCtx, context.Background())

	c := f.Repeat() // unbounded
	// Pull a couple values to be sure the generator goroutine is running.
	<-c
	<-c

	cancel()
	drainUntilClosed(t, c)
}

func TestRepeat_StopsOnServerCancel(t *testing.T) {
	srvCtx, cancel := context.WithCancel(context.Background())
	f := newDotFlush(context.Background(), srvCtx)

	c := f.Repeat() // unbounded
	<-c
	<-c

	cancel()
	drainUntilClosed(t, c)
}

func TestSleep_ReturnsAfterDuration(t *testing.T) {
	f := newDotFlush(context.Background(), context.Background())

	s, err := f.Sleep(1)
	if err != nil {
		t.Fatalf("Sleep error = %v, want nil", err)
	}
	if s != "" {
		t.Errorf("Sleep returned %q, want empty string", s)
	}
}

func TestSleep_RequestCancelReturnsReturnError(t *testing.T) {
	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()
	f := newDotFlush(reqCtx, context.Background())

	// A long sleep would block, but the already-cancelled request context wins.
	if _, err := f.Sleep(60_000); !isReturnError(err) {
		t.Errorf("Sleep error = %v, want ReturnError", err)
	}
}

func TestSleep_ServerCancelReturnsReturnError(t *testing.T) {
	srvCtx, cancel := context.WithCancel(context.Background())
	cancel()
	f := newDotFlush(context.Background(), srvCtx)

	if _, err := f.Sleep(60_000); !isReturnError(err) {
		t.Errorf("Sleep error = %v, want ReturnError", err)
	}
}

func TestWaitForServerStop_RequestCancelReturnsReturnError(t *testing.T) {
	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()
	f := newDotFlush(reqCtx, context.Background())

	// Client disconnect: the handler should abort, signalled by ReturnError.
	if _, err := f.WaitForServerStop(); !isReturnError(err) {
		t.Errorf("WaitForServerStop error = %v, want ReturnError", err)
	}
}

func TestWaitForServerStop_ServerStopReturnsNil(t *testing.T) {
	srvCtx, cancel := context.WithCancel(context.Background())
	cancel()
	f := newDotFlush(context.Background(), srvCtx)

	// Server shutdown: this is a normal completion, not an abort.
	if _, err := f.WaitForServerStop(); err != nil {
		t.Errorf("WaitForServerStop error = %v, want nil", err)
	}
}

func isReturnError(err error) bool {
	_, ok := err.(ReturnError)
	return ok
}

// ---------------------------------------------------------------------------
// Layer 3: end-to-end streaming through the flushing handler.
//
// These go through Instance.ServeHTTP so they cover the real SSE handler:
// the Accept gate, incremental flushing (bytes reach the client between events
// rather than buffered to the end), and cancellation when the client
// disconnects mid-stream. hurl reads the whole body at once and cannot observe
// either property.
// ---------------------------------------------------------------------------

// flushRecorder snapshots the response body on every Flush so a test can assert
// that content is delivered incrementally. It embeds an httptest.ResponseRecorder
// and overrides Flush; the embedded recorder supplies Header/Write/WriteHeader,
// so it still satisfies the handler's flusher interface.
type flushRecorder struct {
	*httptest.ResponseRecorder
	mu        sync.Mutex
	snapshots []string
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (f *flushRecorder) Flush() {
	f.mu.Lock()
	f.snapshots = append(f.snapshots, f.Body.String())
	f.mu.Unlock()
	f.ResponseRecorder.Flush()
}

func (f *flushRecorder) snapshotCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.snapshots)
}

func (f *flushRecorder) firstSnapshot() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.snapshots) == 0 {
		return ""
	}
	return f.snapshots[0]
}

func TestServeHTTP_SSE_RequiresEventStreamAccept(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"feed.html": `{{define "SSE /events"}}{{range .Flush.Repeat 1}}data: {{.}}{{printf "\n\n"}}{{$.Flush.Flush}}{{end}}{{end}}`,
	})

	// Without the SSE Accept header the flushing handler refuses with 406.
	w := doRequest(inst, http.MethodGet, "/events")
	if w.Code != http.StatusNotAcceptable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotAcceptable)
	}
}

func TestServeHTTP_SSE_FlushesIncrementally(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"feed.html": `{{define "SSE /events"}}{{range .Flush.Repeat 2}}data: {{.}}{{printf "\n\n"}}{{$.Flush.Flush}}{{$.Flush.Sleep 5}}{{end}}{{end}}`,
	})

	fr := newFlushRecorder()
	r := httptest.NewRequest(http.MethodGet, "/events", nil)
	r.Header.Set("Accept", "text/event-stream")
	inst.ServeHTTP(fr, r)

	if fr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", fr.Code, http.StatusOK)
	}
	if ct := fr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}

	// Each event is flushed on its own, so we captured several growing snapshots
	// rather than a single end-of-handler dump.
	if n := fr.snapshotCount(); n < 2 {
		t.Fatalf("recorded %d flushes, want at least 2 (incremental delivery)", n)
	}

	// The very first flush carries only the first event.
	if first := fr.firstSnapshot(); first != "data: 0\n\n" {
		t.Errorf("first flush = %q, want %q", first, "data: 0\n\n")
	}
	if first := fr.firstSnapshot(); strings.Contains(first, "data: 1") {
		t.Errorf("first flush %q already contains the second event; content was buffered, not streamed", first)
	}

	// The complete stream still contains every event in order.
	body := fr.Body.String()
	for _, want := range []string{"data: 0", "data: 1", "data: 2"} {
		if !strings.Contains(body, want) {
			t.Errorf("final body = %q, want it to contain %q", body, want)
		}
	}
}

func TestServeHTTP_SSE_StopsWhenClientDisconnects(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		// Unbounded stream: it only ends when a context is cancelled.
		"feed.html": `{{define "SSE /events"}}{{range .Flush.Repeat}}data: tick{{printf "\n\n"}}{{$.Flush.Flush}}{{$.Flush.Sleep 10}}{{end}}{{end}}`,
	})

	fr := newFlushRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	r.Header.Set("Accept", "text/event-stream")

	done := make(chan struct{})
	go func() {
		inst.ServeHTTP(fr, r)
		close(done)
	}()

	// Wait until the stream has actually started emitting before disconnecting.
	waitForCondition(t, 2*time.Second, func() bool { return fr.snapshotCount() > 0 })

	cancel() // simulate the client closing the connection

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not return after client disconnect")
	}
}

// waitForCondition polls cond until it is true or the timeout elapses.
func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}
