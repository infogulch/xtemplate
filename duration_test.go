package xtemplate

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDuration_JSONString(t *testing.T) {
	var d Duration
	if err := json.Unmarshal([]byte(`"45s"`), &d); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d != Duration(45*time.Second) {
		t.Errorf("got %v, want 45s", d)
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != `"45s"` {
		t.Errorf("Marshal = %s, want \"45s\"", b)
	}
}

func TestDuration_JSONNullAndZero(t *testing.T) {
	d := Duration(time.Second)
	if err := json.Unmarshal([]byte(`null`), &d); err != nil {
		t.Fatalf("Unmarshal null: %v", err)
	}
	if d != 0 {
		t.Errorf("null → %v, want 0", d)
	}
	b, err := json.Marshal(Duration(0))
	if err != nil {
		t.Fatalf("Marshal zero: %v", err)
	}
	if string(b) != `"0s"` {
		t.Errorf("Marshal zero = %s, want \"0s\"", b)
	}
}

func TestDuration_Text(t *testing.T) {
	var d Duration
	if err := d.UnmarshalText([]byte("1m30s")); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	if d != Duration(90*time.Second) {
		t.Errorf("got %v, want 90s", d)
	}
	text, err := d.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	if string(text) != "1m30s" {
		t.Errorf("MarshalText = %q, want 1m30s", text)
	}
	if err := d.UnmarshalText([]byte("")); err != nil {
		t.Fatalf("empty UnmarshalText: %v", err)
	}
	if d != 0 {
		t.Errorf("empty → %v, want 0", d)
	}
}

func TestDuration_JSONBad(t *testing.T) {
	var d Duration
	if err := json.Unmarshal([]byte(`true`), &d); err == nil {
		t.Fatal("expected error for boolean JSON")
	}
	if err := json.Unmarshal([]byte(`90000000000`), &d); err == nil {
		t.Fatal("expected error for numeric JSON")
	}
	if err := d.UnmarshalText([]byte("notaduration")); err == nil {
		t.Fatal("expected error for bad text")
	}
}

func TestDuration_InStruct(t *testing.T) {
	type cfg struct {
		Timeout Duration `json:"timeout"`
	}
	var c cfg
	if err := json.Unmarshal([]byte(`{"timeout":"200ms"}`), &c); err != nil {
		t.Fatalf("struct Unmarshal: %v", err)
	}
	if c.Timeout != Duration(200*time.Millisecond) {
		t.Errorf("got %v, want 200ms", c.Timeout)
	}
}
