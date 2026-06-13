package xtemplate

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
