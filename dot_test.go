package xtemplate

import (
	"context"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"text/template"
)

type testGreeting struct {
	Greeting string
}

type testDotProvider struct {
	field string
}

func (p testDotProvider) FieldName() string          { return p.field }
func (p testDotProvider) Init(context.Context) error { return nil }
func (p testDotProvider) Value(Request) (any, error) {
	return testGreeting{Greeting: "hi"}, nil
}

func TestMakeDotHappyPath(t *testing.T) {
	provider := testDotProvider{field: "Test"}
	d, err := makeDot([]DotConfig{provider})
	if err != nil {
		t.Fatalf("unexpected error from makeDot: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)

	val, err := d.value(context.Background(), w, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val == nil {
		t.Fatal("expected a non-nil *reflect.Value")
	}
	if val.Kind() != reflect.Struct {
		t.Fatalf("kind = %v, want struct", val.Kind())
	}
	if val.NumField() != 1 {
		t.Fatalf("NumField = %d, want 1", val.NumField())
	}

	field := val.Type().Field(0)
	if field.Name != "Test" {
		t.Errorf("field name = %q, want %q", field.Name, "Test")
	}
	if field.Type != reflect.TypeOf(testGreeting{}) {
		t.Errorf("field type = %v, want %v", field.Type, reflect.TypeOf(testGreeting{}))
	}

	greeting := val.Field(0).FieldByName("Greeting")
	if !greeting.IsValid() {
		t.Fatal("expected the embedded struct to have a Greeting field")
	}
	if greeting.String() != "hi" {
		t.Errorf("Greeting = %q, want %q", greeting.String(), "hi")
	}
}

// testNilDotProvider is a DotConfig whose Value returns (nil, err), simulating
// a custom provider that fails during type inference.
type testNilDotProvider struct {
	field string
}

func (p testNilDotProvider) FieldName() string          { return p.field }
func (p testNilDotProvider) Init(context.Context) error { return nil }
func (p testNilDotProvider) Value(Request) (any, error) {
	return nil, fmt.Errorf("boom")
}

// cleanupRecorder captures whether Cleanup ran and the error it received.
type cleanupRecorder struct {
	called bool
	gotErr error
}

// recordingCleanupProvider is a CleanupDotProvider that records its Cleanup
// invocation, used to verify partially-constructed dot values are unwound.
type recordingCleanupProvider struct {
	field string
	rec   *cleanupRecorder
}

func (p recordingCleanupProvider) FieldName() string          { return p.field }
func (p recordingCleanupProvider) Init(context.Context) error { return nil }
func (p recordingCleanupProvider) Value(Request) (any, error) { return struct{}{}, nil }
func (p recordingCleanupProvider) Cleanup(_ any, err error) error {
	p.rec.called = true
	p.rec.gotErr = err
	return err
}

var _ CleanupDotProvider = recordingCleanupProvider{}

// failingValueProvider returns a typed value (so makeDot can infer its field
// type) but always fails at request time, simulating a provider whose Value
// errors after earlier providers have already been constructed.
type failingValueProvider struct {
	field string
	err   error
}

func (p failingValueProvider) FieldName() string          { return p.field }
func (p failingValueProvider) Init(context.Context) error { return nil }
func (p failingValueProvider) Value(Request) (any, error) { return struct{}{}, p.err }

// TestDotValuePartialCleanup verifies that when a provider's Value fails partway
// through constructing a dot value, the providers that were already built get
// their Cleanup called (with the construction error) so they don't leak
// resources such as open DB transactions.
func TestDotValuePartialCleanup(t *testing.T) {
	rec := &cleanupRecorder{}
	d, err := makeDot([]DotConfig{
		recordingCleanupProvider{field: "A", rec: rec},
		failingValueProvider{field: "B", err: fmt.Errorf("boom")},
	})
	if err != nil {
		t.Fatalf("unexpected error from makeDot: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	val, err := d.value(context.Background(), w, r)
	if err == nil {
		t.Fatal("expected an error when a later provider's Value fails, got nil")
	}
	if val != nil {
		t.Errorf("expected a nil dot value on error, got %v", val)
	}
	if !rec.called {
		t.Error("expected the earlier provider's Cleanup to run during partial unwind")
	}
	if rec.gotErr == nil {
		t.Error("expected the construction error to be passed to Cleanup")
	}
}

// TestWithArgsOnRealDot exercises withArgs against a dot built by makeDot and
// executed through text/template, covering both wrapping the whole dot and
// scoping down to a single promoted field.
func TestWithArgsOnRealDot(t *testing.T) {
	d, err := makeDot([]DotConfig{testDotProvider{field: "Greeter"}})
	if err != nil {
		t.Fatalf("makeDot: %v", err)
	}
	val, err := d.value(context.Background(), httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("dot.value: %v", err)
	}
	dot := val.Interface()

	render := func(t *testing.T, text string, data any) (string, error) {
		t.Helper()
		tmpl, err := template.New("t").Funcs(xtemplateFuncs).Parse(text)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		var sb strings.Builder
		err = tmpl.Execute(&sb, data)
		return sb.String(), err
	}

	t.Run("wrap whole dot: fields stay promoted, Args added", func(t *testing.T) {
		got, err := render(t, `{{template "sub" withArgs . "title"}}{{define "sub"}}{{.Args | idx 0}}:{{.Greeter.Greeting}}{{end}}`, dot)
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if want := "title:hi"; got != want {
			t.Errorf("rendered %q, want %q", got, want)
		}
	})

	t.Run("wrap a single field: its members promote, others error", func(t *testing.T) {
		got, err := render(t, `{{$a := withArgs .Greeter "foo"}}{{$a.Greeting}}:{{$a.Args | idx 0}}`, dot)
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if want := "hi:foo"; got != want {
			t.Errorf("rendered %q, want %q", got, want)
		}

		if _, err := render(t, `{{$a := withArgs .Greeter "foo"}}{{$a.Greeter}}`, dot); err == nil {
			t.Error("expected an error accessing a field not present on the wrapped value")
		}
	})
}

func TestMakeDotNilProvider(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("makeDot panicked instead of returning an error: %v", r)
		}
	}()

	provider := testNilDotProvider{field: "Test"}
	_, err := makeDot([]DotConfig{provider})
	if err == nil {
		t.Fatal("expected a non-nil error when a provider returns a nil value, got nil")
	}
}
