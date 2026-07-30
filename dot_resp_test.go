package xtemplate

import (
	"fmt"
	"html/template"
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

// TestDotResp_RespondWith_NoTrailingBuffer ensures partial template output is
// discarded; the client receives only the replacement body and status.
func TestDotResp_RespondWith_NoTrailingBuffer(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"rw.html": `BEFORE{{.Resp.RespondWith 404 "not found"}}AFTER`,
	})

	w := doRequest(inst, http.MethodGet, "/rw")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if body := w.Body.String(); body != "not found" {
		t.Errorf("body = %q, want exactly %q (no BEFORE/AFTER buffer)", body, "not found")
	}
}

// TestDotResp_RespondWith_NoSuperfluousWriteHeader guards a single WriteHeader
// after RespondWith's deferred Finalize commit.
func TestDotResp_RespondWith_NoSuperfluousWriteHeader(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"rw.html": `partial{{.Resp.RespondWith 400 "bad request"}}`,
	})

	cw := &countingResponseWriter{rec: httptest.NewRecorder()}
	r := httptest.NewRequest(http.MethodGet, "/rw", nil)
	inst.ServeHTTP(cw, r)

	if cw.writeHeaderCalls != 1 {
		t.Errorf("WriteHeader called %d times, want exactly 1", cw.writeHeaderCalls)
	}
	if cw.rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", cw.rec.Code, http.StatusBadRequest)
	}
	if body := cw.rec.Body.String(); body != "bad request" {
		t.Errorf("body = %q, want %q", body, "bad request")
	}
}

func TestDotResp_RespondWith_EmptyBodyRedirect(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"redir.html": `{{.Resp.AddHeader "Location" "/"}}{{.Resp.RespondWith 303 ""}}`,
	})

	w := doRequest(inst, http.MethodGet, "/redir")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q, want /", loc)
	}
	if body := w.Body.String(); body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	// Empty body must not force a Content-Type (RespondWith does not set one).
	if ct := w.Header().Get("Content-Type"); ct != "" {
		t.Errorf("Content-Type = %q, want unset for empty body", ct)
	}
}

func TestDotResp_RespondWith_HTMLContentTypeDefault(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"page.html": `{{.Resp.RespondWith 404 (.X.Template "/.404.html" .)}}`,
		".404.html": `<h1>Not Found</h1>`,
	})

	w := doRequest(inst, http.MethodGet, "/page")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if body := w.Body.String(); body != "<h1>Not Found</h1>" {
		t.Errorf("body = %q, want HTML page", body)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html…", ct)
	}
}

func TestDotResp_RespondWith_HTMLContentTypeRespectsSet(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"page.html":  `{{.Resp.SetHeader "Content-Type" "text/plain; charset=utf-8"}}{{.Resp.RespondWith 200 (.X.Template "/.frag.html" .)}}`,
		".frag.html": `hello`,
	})

	w := doRequest(inst, http.MethodGet, "/page")
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want explicit text/plain", ct)
	}
}

func TestDotResp_RespondWith_StringNoForcedContentType(t *testing.T) {
	// string body without explicit Content-Type: we do not default to HTML.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	d := DotResp{
		Header: make(http.Header),
		status: http.StatusOK,
		w:      w,
		r:      r,
		log:    GetLogger(r.Context()),
	}
	if _, err := d.RespondWith(400, "name is required"); err == nil {
		t.Fatal("expected ReturnError")
	} else if _, ok := err.(ReturnError); !ok {
		t.Fatalf("err = %v, want ReturnError", err)
	}
	if err := (dotRespProvider{}).Finalize(d, nil); err != errResponseServed {
		t.Fatalf("Finalize err = %v, want errResponseServed", err)
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if body := w.Body.String(); body != "name is required" {
		t.Errorf("body = %q", body)
	}
	// No Content-Type forced by RespondWith for plain strings.
	if ct := w.Header().Get("Content-Type"); ct != "" && strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, must not default to text/html for string body", ct)
	}
}

func TestDotResp_RespondWith_BytesAndUnsupported(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	d := DotResp{
		Header: make(http.Header),
		status: http.StatusOK,
		w:      w,
		r:      r,
		log:    GetLogger(r.Context()),
	}
	if _, err := d.RespondWith(200, []byte("bytes-ok")); err == nil {
		t.Fatal("expected ReturnError")
	} else if _, ok := err.(ReturnError); !ok {
		t.Fatalf("err = %v, want ReturnError", err)
	}
	if err := (dotRespProvider{}).Finalize(d, nil); err != errResponseServed {
		t.Fatalf("Finalize err = %v, want errResponseServed", err)
	}
	if body := w.Body.String(); body != "bytes-ok" {
		t.Errorf("body = %q, want bytes-ok", body)
	}

	d2 := DotResp{Header: make(http.Header), status: http.StatusOK, w: w, r: r, log: GetLogger(r.Context())}
	_, err := d2.RespondWith(200, 123)
	if err == nil || !strings.Contains(err.Error(), "unsupported body type") {
		t.Errorf("err = %v, want unsupported body type", err)
	}
	_, err = d2.RespondWith(200, nil)
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Errorf("nil body err = %v, want nil message", err)
	}
}

func TestDotResp_RespondWith_EmptyHTMLNoForcedContentType(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	d := DotResp{
		Header: make(http.Header),
		status: http.StatusOK,
		w:      w,
		r:      r,
		log:    GetLogger(r.Context()),
	}
	// template.HTML empty — treated like empty body for Content-Type.
	if _, err := d.RespondWith(204, template.HTML("")); err == nil {
		t.Fatal("expected ReturnError")
	}
	if err := (dotRespProvider{}).Finalize(d, nil); err != errResponseServed {
		t.Fatalf("Finalize err = %v", err)
	}
	if ct := w.Header().Get("Content-Type"); ct != "" {
		t.Errorf("Content-Type = %q, want unset for empty body", ct)
	}
}
