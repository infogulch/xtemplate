package xtemplate

import (
	"context"
	"net/http/httptest"
	"reflect"
	"testing"
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
	d := makeDot([]DotConfig{provider})

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
