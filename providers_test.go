package xtemplate

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// testProvider is a minimal Provider shared by package tests (registry, makeDot).
type testProvider struct {
	Name string `json:"name"`
	Val  string `json:"value"`
}

func (p *testProvider) FieldName() string { return p.Name }
func (p *testProvider) Prototype() any    { return "" }
func (p *testProvider) Value(http.ResponseWriter, *http.Request) (any, error) {
	return p.Val, nil
}

func init() {
	Register("_test", func() Provider { return &testProvider{} })
}

func TestResolveProviders_roundtrip(t *testing.T) {
	raw := json.RawMessage(`{"type":"_test","name":"X","value":"hello"}`)
	got, err := resolveProviders([]json.RawMessage{raw})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 provider, got %d", len(got))
	}
	p := got[0].(*testProvider)
	if p.Name != "X" || p.Val != "hello" {
		t.Fatalf("unexpected decoded value: %+v", p)
	}
}

func TestResolveProviders_unknownWellKnown(t *testing.T) {
	// "nats" is not imported by any file in this test binary so it is
	// guaranteed to be unregistered, exercising the well-known hint path.
	raw := json.RawMessage(`{"type":"nats"}`)
	_, err := resolveProviders([]json.RawMessage{raw})
	if err == nil {
		t.Fatal("expected error for unregistered well-known type")
	}
	want := `add it by importing github.com/infogulch/xtemplate/providers/dotnats`
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

func TestResolveProviders_unknownGeneric(t *testing.T) {
	raw := json.RawMessage(`{"type":"bogus"}`)
	_, err := resolveProviders([]json.RawMessage{raw})
	if err == nil {
		t.Fatal("expected error for completely unknown type")
	}
	want := `ensure the provider package that registers type "bogus" is imported`
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

func TestRegister_duplicatePanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	Register("_test", func() Provider { return &testProvider{} })
}
