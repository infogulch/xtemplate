package caddyfile_test

import (
	"encoding/json"
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"

	xtc "github.com/infogulch/xtemplate/caddy"
	_ "github.com/infogulch/xtemplate/providers/dotbus/caddyfile"
)

func parse(t *testing.T, body string) (json.RawMessage, error) {
	t.Helper()
	input := "outer {\n\tprovider bus Bus {\n" + body + "\n\t}\n}"
	h := httpcaddyfile.Helper{}.WithDispenser(caddyfile.NewTestDispenser(input))
	h.Next()       // "outer"
	h.NextBlock(0) // → "provider"
	h.NextArg()    // "bus"
	h.NextArg()    // "Bus"
	mi, err := caddy.GetModule("xtemplate.providers.bus")
	if err != nil {
		t.Fatalf("GetModule: %v", err)
	}
	return mi.New().(xtc.CaddyfileBlockParser).ParseCaddyfile(h)
}

func TestBusCaddyfile_HappyPath(t *testing.T) {
	raw, err := parse(t, "\t\tbuffer 64")
	if err != nil {
		t.Fatalf("ParseCaddyfile: %v", err)
	}
	var got struct {
		Buffer int `json:"buffer"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Buffer != 64 {
		t.Errorf("buffer = %d, want 64", got.Buffer)
	}
}

func TestBusCaddyfile_EmptyBlock(t *testing.T) {
	raw, err := parse(t, "")
	if err != nil {
		t.Fatalf("ParseCaddyfile: %v", err)
	}
	var got struct {
		Buffer int `json:"buffer"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Buffer != 0 {
		t.Errorf("buffer = %d, want 0 (default)", got.Buffer)
	}
}

func TestBusCaddyfile_Errors(t *testing.T) {
	cases := map[string]string{
		"missing value":  "\t\tbuffer",
		"non-integer":    "\t\tbuffer nope",
		"negative":       "\t\tbuffer -1",
		"unknown option": "\t\tbogus 1",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parse(t, body); err == nil {
				t.Errorf("expected error for %q", name)
			}
		})
	}
}
