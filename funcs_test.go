package xtemplate

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/dustin/go-humanize"
)

func TestFuncMarkdown(t *testing.T) {
	t.Run("yaml front matter splits meta and body", func(t *testing.T) {
		doc, err := FuncMarkdown("---\ntitle: Hello\n---\nbody **content**")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if doc.Meta["title"] != "Hello" {
			t.Errorf("Meta[title] = %v, want %q", doc.Meta["title"], "Hello")
		}
		if !strings.Contains(string(doc.Body), "<strong>content</strong>") {
			t.Errorf("Body = %q, want rendered markdown", doc.Body)
		}
	})

	t.Run("toml front matter", func(t *testing.T) {
		doc, err := FuncMarkdown("+++\ntitle = \"Hi\"\n+++\nbody")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if doc.Meta["title"] != "Hi" {
			t.Errorf("Meta[title] = %v, want %q", doc.Meta["title"], "Hi")
		}
	})

	t.Run("no front matter leaves Meta nil", func(t *testing.T) {
		doc, err := FuncMarkdown("just body")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if doc.Meta != nil {
			t.Errorf("Meta = %v, want nil", doc.Meta)
		}
	})

	t.Run("accepts io.Reader", func(t *testing.T) {
		doc, err := FuncMarkdown(strings.NewReader("---\ntitle: R\n---\nbody"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if doc.Meta["title"] != "R" {
			t.Errorf("Meta[title] = %v, want %q", doc.Meta["title"], "R")
		}
	})

	t.Run("unsupported input type errors", func(t *testing.T) {
		if _, err := FuncMarkdown(42); err == nil {
			t.Error("expected an error for unsupported input type")
		}
	})
}

func TestFuncHumanize(t *testing.T) {
	t.Run("size", func(t *testing.T) {
		got, err := FuncHumanize("size", "2048000")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := humanize.Bytes(2048000); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("time", func(t *testing.T) {
		stamp := time.Now().Add(-2 * time.Hour).Format(time.RFC1123Z)
		got, err := FuncHumanize("time", stamp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, "ago") {
			t.Errorf("got %q, expected a relative time containing \"ago\"", got)
		}
	})

	t.Run("bad size input returns error", func(t *testing.T) {
		_, err := FuncHumanize("size", "not-a-number")
		if err == nil {
			t.Error("expected an error for non-numeric size input")
		}
	})

	t.Run("unknown format type returns error", func(t *testing.T) {
		_, err := FuncHumanize("bogus", "x")
		if err == nil {
			t.Error("expected an error for unknown format type")
		}
	})
}

type tryGreeter struct {
	name string
}

func (g tryGreeter) Greet() (string, error) {
	return "hello " + g.name, nil
}

func TestFuncTry(t *testing.T) {
	t.Run("two-return func succeeds", func(t *testing.T) {
		fn := func(s string) (string, error) { return "hi" + s, nil }
		res, err := FuncTry(fn, "!")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.OK() {
			t.Errorf("OK() = false, want true (Error=%v)", res.Error)
		}
		if res.Value != "hi!" {
			t.Errorf("Value = %v, want %q", res.Value, "hi!")
		}
	})

	t.Run("func returns error", func(t *testing.T) {
		fn := func() (string, error) { return "", fmt.Errorf("boom") }
		res, err := FuncTry(fn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.OK() {
			t.Error("OK() = true, want false")
		}
		if res.Error == nil {
			t.Error("expected res.Error to be set")
		}
	})

	t.Run("one-return error func", func(t *testing.T) {
		fn := func() error { return nil }
		res, err := FuncTry(fn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.OK() {
			t.Errorf("OK() = false, want true (Error=%v)", res.Error)
		}
		if res.Value != nil {
			t.Errorf("Value = %v, want nil", res.Value)
		}
	})

	t.Run("method dispatch by name", func(t *testing.T) {
		res, err := FuncTry(tryGreeter{name: "world"}, "Greet")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.OK() {
			t.Errorf("OK() = false, want true (Error=%v)", res.Error)
		}
		if res.Value != "hello world" {
			t.Errorf("Value = %v, want %q", res.Value, "hello world")
		}
	})

	t.Run("nil func errors", func(t *testing.T) {
		_, err := FuncTry(nil)
		if err == nil {
			t.Error("expected an error for nil func")
		}
	})

	t.Run("non-callable without method name errors", func(t *testing.T) {
		_, err := FuncTry(42)
		if err == nil {
			t.Error("expected an error for non-callable value with no method name")
		}
	})
}

func TestFuncIdx(t *testing.T) {
	arr := []string{"a", "b", "c", "d"}
	got, err := FuncIdx(2, arr)
	if err != nil {
		t.Errorf("FuncIdx(2, %v) returned unexpected error: %v", arr, err)
	}
	if got != "c" {
		t.Errorf("FuncIdx(2, %v) = %v, want %q", arr, got, "c")
	}

	if _, err := FuncIdx(99, arr); err == nil {
		t.Errorf("FuncIdx(99, %v) expected an out-of-range error, got nil", arr)
	}

	if _, err := FuncIdx(-1, arr); err == nil {
		t.Errorf("FuncIdx(-1, %v) expected an out-of-range error, got nil", arr)
	}

	if _, err := FuncIdx(0, 42); err == nil {
		t.Errorf("FuncIdx(0, 42) expected a non-indexable error, got nil")
	}
}

func TestFuncWithArgs(t *testing.T) {
	type data struct{ Name string }

	t.Run("non-struct errors", func(t *testing.T) {
		if _, err := FuncWithArgs(42, "a"); err == nil {
			t.Error("expected an error for non-struct dot")
		}
	})

	t.Run("user struct with a Dot field is wrapped, not rewrapped", func(t *testing.T) {
		// A struct that happens to have a field named "Dot" but no Args field
		// must not be mistaken for an existing wrapper (which used to panic).
		type bad struct{ Dot int }
		got, err := FuncWithArgs(bad{Dot: 5}, "a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !isWithArgs(reflect.TypeOf(got)) {
			t.Errorf("expected %T to be a fresh withArgs wrapper", got)
		}
	})

	t.Run("rewrap replaces args without nesting", func(t *testing.T) {
		w1, err := FuncWithArgs(data{Name: "x"}, "a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		w2, err := FuncWithArgs(w1, "b")
		if err != nil {
			t.Fatalf("unexpected error on rewrap: %v", err)
		}
		if got, want := fmt.Sprintf("%T", w1), fmt.Sprintf("%T", w2); got != want {
			t.Errorf("rewrap changed type: %s vs %s (Dot should stay flat)", got, want)
		}
	})

	t.Run("promotes embedded fields and exposes Args in templates", func(t *testing.T) {
		wrapped, err := FuncWithArgs(data{Name: "world"}, "hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tmpl := template.Must(template.New("t").Funcs(xtemplateFuncs).
			Parse(`{{.Args | idx 0}} {{.Name}}`))
		var sb strings.Builder
		if err := tmpl.Execute(&sb, wrapped); err != nil {
			t.Fatalf("execute: %v", err)
		}
		if got, want := sb.String(), "hello world"; got != want {
			t.Errorf("rendered %q, want %q", got, want)
		}
	})
}
