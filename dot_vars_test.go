package xtemplate

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestDotVars_Methods(t *testing.T) {
	v := make(DotVars)
	if _, err := v.Set("list", "row-1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := v.Get("list")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "row-1" {
		t.Errorf("Get = %v, want row-1", got)
	}
	has, err := v.Has("list")
	if err != nil || !has {
		t.Errorf("Has = %v, %v; want true, nil", has, err)
	}
	missing, err := v.Get("missing")
	if err != nil {
		t.Fatalf("Get missing: %v", err)
	}
	if missing != nil {
		t.Errorf("Get missing = %v, want nil", missing)
	}
	has, err = v.Has("missing")
	if err != nil || has {
		t.Errorf("Has missing = %v, %v; want false, nil", has, err)
	}
	if _, err := v.Delete("list"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if has, _ := v.Has("list"); has {
		t.Error("Has after Delete: want false")
	}
}

func TestDotVars_EmptyKey(t *testing.T) {
	v := make(DotVars)
	if _, err := v.Set("", 1); err == nil {
		t.Error("Set empty key: want error")
	}
	if _, err := v.Get(""); err == nil {
		t.Error("Get empty key: want error")
	}
	if _, err := v.Has(""); err == nil {
		t.Error("Has empty key: want error")
	}
	if _, err := v.Delete(""); err == nil {
		t.Error("Delete empty key: want error")
	}
}

func TestDotVars_CrossTemplateSetGet(t *testing.T) {
	// Hidden basenames are not given path routes; defines register the handlers.
	inst := buildInstance(t, map[string]string{
		".templates.html": `
{{define "load"}}{{.Vars.Set "item" "from-load"}}{{end}}
{{define "GET /page"}}
{{- template "load" . -}}
{{- if .Vars.Has "item" -}}got:{{.Vars.Get "item"}}{{- else -}}missing{{- end -}}
{{end}}
`,
	})

	w := doRequest(inst, http.MethodGet, "/page")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	if body := w.Body.String(); body != "got:from-load" {
		t.Errorf("body = %q, want got:from-load", body)
	}
}

func TestDotVars_NonNilEmptyMapEachRequest(t *testing.T) {
	// Value must return a non-nil empty map so Set does not panic (nil map
	// assignment panics; empty maps are falsey in {{if}} so we probe via Set).
	inst := buildInstance(t, map[string]string{
		"a.html": `{{.Vars.Set "t" 1}}{{len .Vars}}`,
	})
	w := doRequest(inst, http.MethodGet, "/a")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body %q", w.Code, w.Body.String())
	}
	if body := w.Body.String(); body != "1" {
		t.Errorf("body = %q, want 1 (Set on fresh .Vars must work)", body)
	}

	// Direct provider check: non-nil, empty, distinct per Value call.
	p := dotVarsProvider{}
	a, err := p.Value(nil, httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Value(nil, httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	ma, ok := a.(DotVars)
	if !ok || ma == nil {
		t.Fatalf("Value = %T %v, want non-nil DotVars", a, a)
	}
	mb, ok := b.(DotVars)
	if !ok || mb == nil {
		t.Fatalf("Value = %T %v, want non-nil DotVars", b, b)
	}
	if len(ma) != 0 || len(mb) != 0 {
		t.Errorf("fresh maps should be empty, got len %d and %d", len(ma), len(mb))
	}
	ma["x"] = 1
	if _, ok := mb["x"]; ok {
		t.Error("Value must return a distinct map per call")
	}
}

func TestDotVars_RequestIsolation(t *testing.T) {
	// Concurrent requests must not share the Vars map.
	inst := buildInstance(t, map[string]string{
		".routes.html": `
{{define "GET /set/{v}"}}
{{- .Vars.Set "k" (.Req.PathValue "v") -}}
{{- /* small window where another request could clobber a shared map */ -}}
{{- .Vars.Get "k" -}}
{{end}}
`,
	})

	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want := fmt.Sprintf("v%d", i)
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/set/"+want, nil)
			inst.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				errs <- fmt.Errorf("status %d for %s", w.Code, want)
				return
			}
			if got := w.Body.String(); got != want {
				errs <- fmt.Errorf("body = %q, want %q (map shared across requests?)", got, want)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestDotVars_OnFlushingDot(t *testing.T) {
	// .Vars is present on flushing handlers as well as buffered.
	inst := buildInstance(t, map[string]string{
		".sse.html": `
{{define "SSE /events"}}
{{- .Vars.Set "x" "1" -}}
{{- .Vars.Get "x" -}}
{{end}}
`,
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/events", nil)
	r.Header.Set("Accept", "text/event-stream")
	inst.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body %q", w.Code, w.Body.String())
	}
	if body := w.Body.String(); body != "1" {
		t.Errorf("body = %q, want 1", body)
	}
}
