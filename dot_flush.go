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

type flushDotProvider struct{}

func (flushDotProvider) Type() reflect.Type { return reflect.TypeOf(FlushDot{}) }

func (flushDotProvider) Value(_ *slog.Logger, sctx context.Context, w http.ResponseWriter, r *http.Request) (reflect.Value, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return reflect.Value{}, fmt.Errorf("response writer could not cast to http.Flusher")
	}
	return reflect.ValueOf(FlushDot{flusher: f, serverCtx: sctx, requestCtx: r.Context()}), nil
}

func (flushDotProvider) Cleanup(v reflect.Value, err error) {
	if err == nil {
		v.Interface().(FlushDot).flusher.Flush()
	}
}

var _ DotProvider = flushDotProvider{}

type FlushDot struct {
	flusher               http.Flusher
	serverCtx, requestCtx context.Context
}

func (f FlushDot) Flush() string {
	f.flusher.Flush()
	return ""
}

func (f FlushDot) Repeat(max_ ...int) <-chan int {
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
func (f FlushDot) Sleep(ms int) (string, error) {
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
func (f FlushDot) WaitForServerStop() (string, error) {
	select {
	case <-f.requestCtx.Done():
		return "", ReturnError{}
	case <-f.serverCtx.Done():
		return "", nil
	}
}
