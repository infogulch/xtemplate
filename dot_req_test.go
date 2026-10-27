package xtemplate

import (
	"errors"
	"html/template"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func dotReqWithBody(body io.Reader) *DotReq {
	r := httptest.NewRequest(http.MethodPost, "/", body)
	if body == nil {
		r.Body = nil
	}
	return &DotReq{Request: r}
}

func postBody(inst *Instance, target, contentType, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	inst.ServeHTTP(w, r)
	return w
}

func assertErrorStatus(t *testing.T, err error, want int) {
	t.Helper()
	var es ErrorStatus
	if !errors.As(err, &es) {
		t.Fatalf("error = %v, want ErrorStatus(%d)", err, want)
	}
	if int(es) != want {
		t.Fatalf("ErrorStatus = %d, want %d (err %v)", int(es), want, err)
	}
}

func TestDotReq_ReadBody_NilAndEmpty(t *testing.T) {
	var nilReq *DotReq
	got, err := nilReq.ReadBody()
	if err != nil || got != "" {
		t.Fatalf("nil receiver: got %q, %v; want empty, nil", got, err)
	}

	got, err = (&DotReq{}).ReadBody()
	if err != nil || got != "" {
		t.Fatalf("nil request: got %q, %v; want empty, nil", got, err)
	}

	got, err = dotReqWithBody(nil).ReadBody()
	if err != nil || got != "" {
		t.Fatalf("nil body: got %q, %v; want empty, nil", got, err)
	}

	got, err = dotReqWithBody(strings.NewReader("")).ReadBody()
	if err != nil || got != "" {
		t.Fatalf("empty reader: got %q, %v; want empty, nil", got, err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = http.NoBody
	got, err = (&DotReq{Request: req}).ReadBody()
	if err != nil || got != "" {
		t.Fatalf("http.NoBody: got %q, %v; want empty, nil", got, err)
	}
}

func TestDotReq_ReadBody_TextAndLimit(t *testing.T) {
	const payload = `{"name":"ada","n":3}`
	req := dotReqWithBody(strings.NewReader(payload))

	got, err := req.ReadBody()
	if err != nil {
		t.Fatalf("ReadBody: %v", err)
	}
	if got != payload {
		t.Errorf("ReadBody = %q, want %q", got, payload)
	}

	got, err = req.ReadBody(512)
	if err != nil {
		t.Fatalf("ReadBody(512): %v", err)
	}
	if got != payload {
		t.Errorf("ReadBody(512) = %q, want %q", got, payload)
	}

	// The restored body is readable by something other than ReadBody.
	rest, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("restored body read: %v", err)
	}
	if string(rest) != payload {
		t.Errorf("restored body = %q, want %q", rest, payload)
	}
}

func TestDotReq_ReadBody_ExactAndTooLarge(t *testing.T) {
	exact := dotReqWithBody(strings.NewReader("abcd"))
	got, err := exact.ReadBody(4)
	if err != nil {
		t.Fatalf("exact limit: %v", err)
	}
	if got != "abcd" || len(got) != 4 {
		t.Fatalf("exact limit = %q (len %d), want %q", got, len(got), "abcd")
	}

	over := dotReqWithBody(strings.NewReader("abcde"))
	got, err = over.ReadBody(4)
	if got != "" {
		t.Errorf("over limit body = %q, want empty", got)
	}
	assertErrorStatus(t, err, http.StatusRequestEntityTooLarge)
	if err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("over limit error = %v, want a descriptive exceed-limit error", err)
	}

	// The rejected read is rewound, so a later call with a sufficient limit
	// still returns the full payload.
	got, err = over.ReadBody(5)
	if err != nil {
		t.Fatalf("retry after 413: %v", err)
	}
	if got != "abcde" {
		t.Errorf("retry after 413 = %q, want abcde", got)
	}
}

func TestDotReq_ReadBody_DefaultLimit(t *testing.T) {
	if DefaultMaxBodyBytes != 1<<20 {
		t.Fatalf("DefaultMaxBodyBytes = %d, want %d", DefaultMaxBodyBytes, 1<<20)
	}

	exact := strings.Repeat("a", int(DefaultMaxBodyBytes))
	got, err := dotReqWithBody(strings.NewReader(exact)).ReadBody()
	if err != nil {
		t.Fatalf("default exact: %v", err)
	}
	if got != exact {
		t.Fatalf("default exact len = %d, want %d", len(got), len(exact))
	}

	_, err = dotReqWithBody(strings.NewReader(exact + "b")).ReadBody()
	assertErrorStatus(t, err, http.StatusRequestEntityTooLarge)
}

func TestDotReq_ReadBody_InvalidArgs(t *testing.T) {
	req := dotReqWithBody(strings.NewReader("hello"))
	cases := []struct {
		name string
		args []int64
		want string
	}{
		{name: "zero", args: []int64{0}, want: "positive"},
		{name: "negative", args: []int64{-1}, want: "positive"},
		{name: "two", args: []int64{8, 16}, want: "at most one"},
		{name: "two with non-positive", args: []int64{0, 16}, want: "at most one"},
		{name: "overflow", args: []int64{math.MaxInt64}, want: "too large"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := req.ReadBody(tc.args...)
			if err == nil {
				t.Fatal("expected error")
			}
			if got != "" {
				t.Errorf("body = %q, want empty", got)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.want)
			}
			var es ErrorStatus
			if errors.As(err, &es) {
				t.Errorf("invalid args wrapped ErrorStatus %d; want a plain error", int(es))
			}
		})
	}

	// Invalid arguments are rejected before the body is consumed.
	got, err := req.ReadBody()
	if err != nil {
		t.Fatalf("body consumed by invalid call: %v", err)
	}
	if got != "hello" {
		t.Errorf("body after invalid args = %q, want hello", got)
	}
}

func TestDotReq_ReadBody_ReadError(t *testing.T) {
	req := &DotReq{Request: httptest.NewRequest(http.MethodPost, "/", nil)}
	req.Body = io.NopCloser(errReader{errors.New("boom")})
	got, err := req.ReadBody(32)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v, want boom", err)
	}
	if got != "" {
		t.Errorf("body = %q, want empty", got)
	}
	var es ErrorStatus
	if errors.As(err, &es) {
		t.Errorf("read error wrapped ErrorStatus %d", int(es))
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestDotReq_ReadBody_Templates(t *testing.T) {
	inst := buildInstance(t, map[string]string{
		"routes.html": `
{{define "POST /raw"}}{{.Req.ReadBody}}{{end}}
{{define "POST /limited"}}{{.Req.ReadBody 512}}{{end}}
{{define "POST /json"}}{{with .Req.ReadBody | fromJson}}{{.name}}-{{.n}}{{end}}{{end}}
{{define "POST /empty"}}[{{.Req.ReadBody}}]{{end}}
{{define "POST /exact"}}{{.Req.ReadBody 4}}{{end}}
{{define "POST /zero"}}{{.Req.ReadBody 0}}{{end}}
{{define "POST /neg"}}{{.Req.ReadBody -1}}{{end}}
{{define "POST /two"}}{{.Req.ReadBody 8 16}}{{end}}
{{define "POST /twice"}}{{.Req.ReadBody}}|{{.Req.ReadBody 1024}}{{end}}
{{define "POST /twice-small"}}{{.Req.ReadBody 100}}|{{.Req.ReadBody 3}}{{end}}
{{define "POST /form"}}{{.Req.ReadBody}}|{{.Req.FormValue "name"}}{{end}}
`,
	})

	const payload = `{"name":"ada","n":3}`
	// html/template escapes the printed string; the JSON route below checks the
	// unescaped value via fromJson.
	escaped := template.HTMLEscapeString(payload)
	w := postBody(inst, "/raw", "application/json", payload)
	if w.Code != http.StatusOK || w.Body.String() != escaped {
		t.Fatalf("POST /raw = %d %q, want 200 %q", w.Code, w.Body.String(), escaped)
	}

	w = postBody(inst, "/limited", "application/json", payload)
	if w.Code != http.StatusOK || w.Body.String() != escaped {
		t.Fatalf("POST /limited = %d %q, want 200 %q", w.Code, w.Body.String(), escaped)
	}

	w = postBody(inst, "/json", "application/json", payload)
	if w.Code != http.StatusOK || w.Body.String() != "ada-3" {
		t.Fatalf("POST /json = %d %q, want 200 ada-3", w.Code, w.Body.String())
	}

	w = postBody(inst, "/empty", "text/plain", "")
	if w.Code != http.StatusOK || w.Body.String() != "[]" {
		t.Fatalf("POST /empty = %d %q, want 200 []", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	inst.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/empty", nil))
	if w.Code != http.StatusOK || w.Body.String() != "[]" {
		t.Fatalf("POST /empty nil body = %d %q, want 200 []", w.Code, w.Body.String())
	}

	w = postBody(inst, "/exact", "text/plain", "abcd")
	if w.Code != http.StatusOK || w.Body.String() != "abcd" {
		t.Fatalf("POST /exact = %d %q, want 200 abcd", w.Code, w.Body.String())
	}

	w = postBody(inst, "/exact", "text/plain", "abcde")
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("POST /exact over = %d, want %d (body %q)", w.Code, http.StatusRequestEntityTooLarge, w.Body.String())
	}
	if strings.Contains(strings.ToLower(w.Body.String()), "internal server error") {
		t.Errorf("413 body = %q, must not be internal server error", w.Body.String())
	}

	for _, target := range []string{"/zero", "/neg", "/two"} {
		w = postBody(inst, target, "text/plain", "hello")
		if w.Code != http.StatusInternalServerError {
			t.Errorf("POST %s = %d, want 500 (body %q)", target, w.Code, w.Body.String())
		}
	}

	w = postBody(inst, "/twice", "text/plain", "hello")
	if w.Code != http.StatusOK || w.Body.String() != "hello|hello" {
		t.Fatalf("POST /twice = %d %q, want 200 hello|hello", w.Code, w.Body.String())
	}

	// First call accepts the body; the second call's smaller limit must 413
	// and must not flush the partial template buffer.
	w = postBody(inst, "/twice-small", "text/plain", "abcdef")
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("POST /twice-small = %d, want 413 (body %q)", w.Code, w.Body.String())
	}
	if w.Body.String() != "" {
		t.Errorf("POST /twice-small body = %q, want empty", w.Body.String())
	}

	w = postBody(inst, "/form", "application/x-www-form-urlencoded", "name=ada")
	if w.Code != http.StatusOK || w.Body.String() != "name=ada|ada" {
		t.Fatalf("POST /form = %d %q, want 200 name=ada|ada", w.Code, w.Body.String())
	}
}
