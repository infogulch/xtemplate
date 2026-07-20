package xtemplate

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestMakeDotHappyPath(t *testing.T) {
	// Reuse testProvider from providers_test.go (same package).
	provider := &testProvider{Name: "Test", Val: "hi"}
	d, err := makeDot([]Provider{provider})
	if err != nil {
		t.Fatalf("unexpected error from makeDot: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)

	val, err := d.value(w, r)
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
	if field.Type != reflect.TypeOf("") {
		t.Errorf("field type = %v, want string", field.Type)
	}
	if got := val.Field(0).String(); got != "hi" {
		t.Errorf("field value = %q, want %q", got, "hi")
	}
}

// testNilPrototypeProvider is a Provider whose Prototype returns nil.
type testNilPrototypeProvider struct {
	field string
}

func (p testNilPrototypeProvider) FieldName() string { return p.field }
func (p testNilPrototypeProvider) Prototype() any    { return nil }
func (p testNilPrototypeProvider) Value(http.ResponseWriter, *http.Request) (any, error) {
	return nil, fmt.Errorf("boom")
}

// finalizeRecorder captures whether Finalize ran and the error it received.
type finalizeRecorder struct {
	called bool
	gotErr error
}

// recordingFinalizeProvider is a Finalizer that records its Finalize
// invocation, used to verify partially-constructed dot values are unwound.
type recordingFinalizeProvider struct {
	field string
	rec   *finalizeRecorder
}

func (p recordingFinalizeProvider) FieldName() string { return p.field }
func (p recordingFinalizeProvider) Prototype() any    { return struct{}{} }
func (p recordingFinalizeProvider) Value(http.ResponseWriter, *http.Request) (any, error) {
	return struct{}{}, nil
}
func (p recordingFinalizeProvider) Finalize(_ any, err error) error {
	p.rec.called = true
	p.rec.gotErr = err
	return err
}

var _ Finalizer = recordingFinalizeProvider{}

// failingValueProvider returns a typed prototype but always fails at request
// time, simulating a provider whose Value errors after earlier providers have
// already been constructed.
type failingValueProvider struct {
	field string
	err   error
}

func (p failingValueProvider) FieldName() string { return p.field }
func (p failingValueProvider) Prototype() any    { return struct{}{} }
func (p failingValueProvider) Value(http.ResponseWriter, *http.Request) (any, error) {
	return struct{}{}, p.err
}

// TestDotValuePartialFinalize verifies that when a provider's Value fails
// partway through constructing a dot value, the providers that were already
// built get their Finalize called (with the construction error) so they don't
// leak resources such as open DB transactions.
func TestDotValuePartialFinalize(t *testing.T) {
	rec := &finalizeRecorder{}
	d, err := makeDot([]Provider{
		recordingFinalizeProvider{field: "A", rec: rec},
		failingValueProvider{field: "B", err: fmt.Errorf("boom")},
	})
	if err != nil {
		t.Fatalf("unexpected error from makeDot: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	val, err := d.value(w, r)
	if err == nil {
		t.Fatal("expected an error when a later provider's Value fails, got nil")
	}
	if val != nil {
		t.Errorf("expected a nil dot value on error, got %v", val)
	}
	if !rec.called {
		t.Error("expected the earlier provider's Finalize to run during partial unwind")
	}
	if rec.gotErr == nil {
		t.Error("expected the construction error to be passed to Finalize")
	}
}

func TestMakeDotNilPrototype(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("makeDot panicked instead of returning an error: %v", r)
		}
	}()

	provider := testNilPrototypeProvider{field: "Test"}
	_, err := makeDot([]Provider{provider})
	if err == nil {
		t.Fatal("expected a non-nil error when Prototype returns nil, got nil")
	}
}
