package xtemplate

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"
)

type dotFlushProvider struct{}

func (dotFlushProvider) Value(r Request) (any, error) {
	f, ok := r.W.(http.Flusher)
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

// DotFlush is used as the .Flush field for flushing template handlers (SSE).
type DotFlush struct {
	flusher               http.Flusher
	serverCtx, requestCtx context.Context
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
