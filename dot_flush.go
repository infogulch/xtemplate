package xtemplate

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"reflect"
	"time"
)

type dotFlushProvider struct{}

func (dotFlushProvider) Type() reflect.Type { return reflect.TypeOf(&DotFlush{}) }

func (dotFlushProvider) Value(_ *slog.Logger, sctx context.Context, w http.ResponseWriter, r *http.Request) (reflect.Value, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return reflect.Value{}, fmt.Errorf("response writer could not cast to http.Flusher")
	}
	return reflect.ValueOf(&DotFlush{flusher: f, serverCtx: sctx, requestCtx: r.Context()}), nil
}

func (dotFlushProvider) Cleanup(v reflect.Value, err error) {
	if err == nil {
		v.Interface().(DotFlush).flusher.Flush()
	}
}

var _ DotProvider = dotFlushProvider{}

// DotFlush is used as the `.Flush` field for flushing template handlers (SSE).
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

// Block blocks execution until the request is canceled by the client or until
// the server closes.
func (f *DotFlush) WaitForServerStop() (string, error) {
	select {
	case <-f.requestCtx.Done():
		return "", ReturnError{}
	case <-f.serverCtx.Done():
		return "", nil
	}
}
