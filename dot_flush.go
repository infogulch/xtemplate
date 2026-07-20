package xtemplate

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

// dotFlushProvider contributes .Flush. It stores the instance context in Init
// so Sleep/Repeat/WaitForServerStop can observe reload/stop without receiving
// that context on every Value call.
type dotFlushProvider struct {
	serverCtx context.Context
}

func (dotFlushProvider) FieldName() string { return "Flush" }
func (dotFlushProvider) Prototype() any    { return &DotFlush{} }

func (p *dotFlushProvider) Init(ctx context.Context) error {
	p.serverCtx = ctx
	return nil
}

func (p *dotFlushProvider) Value(w http.ResponseWriter, r *http.Request) (any, error) {
	f, ok := w.(flusher)
	if !ok {
		return &DotFlush{}, fmt.Errorf("response writer could not cast to http.Flusher")
	}
	return &DotFlush{flusher: f, serverCtx: p.serverCtx, requestCtx: r.Context()}, nil
}

func (dotFlushProvider) Finalize(v any, err error) error {
	if err == nil {
		v.(*DotFlush).flusher.Flush()
	}
	return err
}

var (
	_ Initializer = (*dotFlushProvider)(nil)
	_ Finalizer   = (*dotFlushProvider)(nil)
)

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
		_, _ = fmt.Fprintf(f.flusher, "event: %s\n", strings.SplitN(event, "\n", 2)[0])
		written = true
	}
	if data != "" {
		for _, line := range strings.Split(data, "\n") {
			_, _ = fmt.Fprintf(f.flusher, "data: %s\n", line)
			written = true
		}
	}
	if id != "" {
		_, _ = fmt.Fprintf(f.flusher, "id: %s\n", strings.SplitN(id, "\n", 2)[0])
		written = true
	}
	if retry != "" {
		_, _ = fmt.Fprintf(f.flusher, "retry: %s\n", strings.SplitN(retry, "\n", 2)[0])
		written = true
	}
	if written {
		_, _ = fmt.Fprintf(f.flusher, "\n\n")
		f.flusher.Flush()
	}
	return nil
}

// Flush flushes any content waiting to be written to the client.
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

// Sleep sleeps for ms milliseconds. Template execution is aborted if the
// request is canceled or the server receives a stop signal.
func (f *DotFlush) Sleep(ms int) (string, error) {
	select {
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return "", nil
	case <-f.requestCtx.Done():
		return "", ReturnError{}
	case <-f.serverCtx.Done():
		return "", ReturnError{}
	}
}

// WaitForServerStop blocks template execution until the server receives a stop
// signal, then continues to allow sending a final response before the request
// is closed. Template execution is aborted if the client cancels the request.
func (f *DotFlush) WaitForServerStop() (string, error) {
	select {
	case <-f.requestCtx.Done():
		return "", ReturnError{}
	case <-f.serverCtx.Done():
		return "", nil
	}
}
