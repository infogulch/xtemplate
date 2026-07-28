package xtemplate

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// countingResponseWriter wraps an httptest.ResponseRecorder and counts how many
// times WriteHeader is called. httptest.ResponseRecorder silently ignores a
// second WriteHeader call, so it cannot by itself observe a superfluous
// WriteHeader; counting the calls here lets the test detect the bug.
type countingResponseWriter struct {
	rec              *httptest.ResponseRecorder
	writeHeaderCalls int
}

func (c *countingResponseWriter) Header() http.Header { return c.rec.Header() }

func (c *countingResponseWriter) Write(b []byte) (int, error) { return c.rec.Write(b) }

func (c *countingResponseWriter) WriteHeader(statusCode int) {
	c.writeHeaderCalls++
	c.rec.WriteHeader(statusCode)
}

// TestDotResp_ServeContent_NoSuperfluousWriteHeader guards against a second
// WriteHeader call after DotResp.ServeContent has already fully written the
// response. http.ServeContent calls WriteHeader exactly once for a normal 200
// serve; without the `served` short-circuit in dotRespProvider.Cleanup, the
// cleanup path would call WriteHeader a second time (total of 2). With the fix
// the total is exactly 1.
func TestDotResp_ServeContent_NoSuperfluousWriteHeader(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"serve.html": `{{.Resp.ServeContent "test.txt" now "hello world"}}`,
	})

	cw := &countingResponseWriter{rec: httptest.NewRecorder()}
	r := httptest.NewRequest(http.MethodGet, "/serve", nil)
	inst.ServeHTTP(cw, r)

	if cw.writeHeaderCalls != 1 {
		t.Errorf("WriteHeader called %d times, want exactly 1 (a second call indicates the superfluous WriteHeader bug)", cw.writeHeaderCalls)
	}
	if cw.rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", cw.rec.Code, http.StatusOK)
	}
	if body := cw.rec.Body.String(); !strings.Contains(body, "hello world") {
		t.Errorf("body = %q, want it to contain %q", body, "hello world")
	}
}

// TestDotResp_ServeContent_NoTrailingBuffer ensures the buffered handler does
// not append template buffer bytes after ServeContent has written the body.
func TestDotResp_ServeContent_NoTrailingBuffer(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		// Content before ServeContent would previously be written after the
		// ServeContent body, corrupting Content-Length / response.
		"serve.html": `BEFORE{{.Resp.ServeContent "test.txt" now "hello world"}}AFTER`,
	})

	w := doRequest(inst, http.MethodGet, "/serve")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if body != "hello world" {
		t.Errorf("body = %q, want exactly %q (no BEFORE/AFTER buffer)", body, "hello world")
	}
}

func TestDotResp_ServeContent_Bytes(t *testing.T) {
	// Drive via provider that calls ServeContent with []byte — template string
	// path is covered above; unit-test the type switch for []byte / bad types.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	d := DotResp{
		Header: make(http.Header),
		status: http.StatusOK,
		w:      w,
		r:      r,
		log:    GetLogger(r.Context()),
	}
	if _, err := d.ServeContent("b.bin", time.Time{}, []byte("bytes-ok")); err == nil {
		// ServeContent returns ReturnError on success
		t.Fatal("expected ReturnError")
	} else if _, ok := err.(ReturnError); !ok {
		t.Fatalf("err = %v, want ReturnError", err)
	}
	if !strings.Contains(w.Body.String(), "bytes-ok") {
		t.Errorf("body = %q, want bytes-ok", w.Body.String())
	}
}

func TestDotResp_ServeContent_UnsupportedType(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	d := DotResp{
		Header: make(http.Header),
		status: http.StatusOK,
		w:      w,
		r:      r,
		log:    GetLogger(r.Context()),
	}
	_, err := d.ServeContent("x", time.Time{}, 123)
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
	if !strings.Contains(err.Error(), "unsupported content type") {
		t.Errorf("err = %v, want unsupported content type", err)
	}
	_, err = d.ServeContent("x", time.Time{}, nil)
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Errorf("nil content err = %v, want nil message", err)
	}
}

// TestErrorStatus_PreservesClientStatus ensures cleanup does not replace an
// intentional ErrorStatus (e.g. 400 from uploads) with http.Error(500).
func TestErrorStatus_PreservesClientStatus(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"bad.html": `{{errorstatus 400}}`,
	}, WithFuncMaps(map[string]any{
		// Last return must be error for template execution to abort.
		"errorstatus": func(code int) (string, error) {
			return "", fmt.Errorf("client: %w", ErrorStatus(code))
		},
	}))

	w := doRequest(inst, http.MethodGet, "/bad")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body %q)", w.Code, http.StatusBadRequest, w.Body.String())
	}
	// Must not be the generic 500 body from http.Error.
	if strings.Contains(strings.ToLower(w.Body.String()), "internal server error") {
		t.Errorf("body = %q, must not be internal server error", w.Body.String())
	}
}
