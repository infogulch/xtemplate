package xtemplate

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

type dotFlushProvider struct{}

func (dotFlushProvider) FieldName() string            { return "Flush" }
func (dotFlushProvider) Init(_ context.Context) error { return nil }
func (dotFlushProvider) Value(r Request) (any, error) {
	f, ok := r.W.(flusher)
	if !ok {
		return &DotFlush{}, fmt.Errorf("response writer could not cast to http.Flusher")
	}
	return &DotFlush{flusher: f, serverCtx: r.ServerCtx, requestCtx: r.R.Context()}, nil
}

func (dotFlushProvider) Cleanup(v any, err error) error {
	if err == nil {
		v.(*DotFlush).flusher.Flush()
	}
	return err
}

var _ CleanupDotProvider = dotFlushProvider{}

type flusher interface {
	http.ResponseWriter
	http.Flusher
}

// DotFlush is used as the .Flush field for flushing template handlers (SSE).
type DotFlush struct {
	flusher               flusher
	serverCtx, requestCtx context.Context
}

// SendSSE sends an sse message by formatting the provided args as an sse event:
//
// Requires 1-4 args: event, data, id, retry
func (f *DotFlush) SendSSE(args ...string) error {
	var event, data, id, retry string
	switch len(args) {
	case 4:
		retry = args[3]
		fallthrough
	case 3:
		id = args[2]
		fallthrough
	case 2:
		data = args[1]
		fallthrough
	case 1:
		event = args[0]
	default:
		return fmt.Errorf("wrong number of args provided. got %d, need 1-4", len(args))
	}
	written := false
	if event != "" {
		fmt.Fprintf(f.flusher, "event: %s\n", strings.SplitN(event, "\n", 2)[0])
		written = true
	}
	if data != "" {
		for _, line := range strings.Split(data, "\n") {
			fmt.Fprintf(f.flusher, "data: %s\n", line)
			written = true
		}
	}
	if id != "" {
		fmt.Fprintf(f.flusher, "id: %s\n", strings.SplitN(id, "\n", 2)[0])
		written = true
	}
	if retry != "" {
		fmt.Fprintf(f.flusher, "retry: %s\n", strings.SplitN(retry, "\n", 2)[0])
		written = true
	}
	if written {
		fmt.Fprintf(f.flusher, "\n\n")
		f.flusher.Flush()
	}
	return nil
}

// Flush flushes any content waiting to written to the client.
func (f *DotFlush) Flush() string {
	f.flusher.Flush()
	return ""
}

// Repeat generates numbers up to max, using math.MaxInt64 if no max is provided.
func (f *DotFlush) Repeat(max_ ...int) <-chan int {
	max := math.MaxInt64 // sorry you can only loop for 2^63-1 iterations max
	if len(max_) > 0 {
		max = max_[0]
	}
	c := make(chan int)
	go func() {
		i := 0
	loop:
		for {
			select {
			case <-f.requestCtx.Done():
				break loop
			case <-f.serverCtx.Done():
				break loop
			case c <- i:
			}
			if i >= max {
				break
			}
			i++
		}
		close(c)
	}()
	return c
}

// Sleep sleeps for ms millisecionds.
func (f *DotFlush) Sleep(ms int) (string, error) {
	select {
	case <-time.After(time.Duration(ms) * time.Millisecond):
	case <-f.requestCtx.Done():
		return "", ReturnError{}
	case <-f.serverCtx.Done():
		return "", ReturnError{}
	}
	return "", nil
}

// WaitForServerStop blocks execution until the request is canceled by the
// client or until the server closes.
func (f *DotFlush) WaitForServerStop() (string, error) {
	select {
	case <-f.requestCtx.Done():
		return "", ReturnError{}
	case <-f.serverCtx.Done():
		return "", nil
	}
}
