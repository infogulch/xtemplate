package xtemplate

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
)

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
