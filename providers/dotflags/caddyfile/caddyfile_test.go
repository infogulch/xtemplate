package caddyfile_test

import (
	"encoding/json"
	"testing"

	caddy "github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"

	xtc "github.com/infogulch/xtemplate/caddy"
	_ "github.com/infogulch/xtemplate/providers/dotflags/caddyfile"
)

func parse(t *testing.T, body string) (json.RawMessage, error) {
	t.Helper()
	input := "outer {\n\tprovider flags Flags {\n" + body + "\n\t}\n}"
	h := httpcaddyfile.Helper{}.WithDispenser(caddyfile.NewTestDispenser(input))
	h.Next()       // "outer"
	h.NextBlock(0) // → "provider"
	h.NextArg()    // "flags"
	h.NextArg()    // "Flags"
	mi, err := caddy.GetModule("xtemplate.providers.flags")
	if err != nil {
		t.Fatalf("GetModule: %v", err)
	}
	return mi.New().(xtc.CaddyfileProvider).ParseCaddyfile(h)
}

func TestFlagsCaddyfile_HappyPath(t *testing.T) {
	raw, err := parse(t, "\t\tenv production\n\t\tversion 1.2.3")
	if err != nil {
		t.Fatalf("ParseCaddyfile: %v", err)
	}
	var got struct {
		Values map[string]string `json:"values"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Values["env"] != "production" {
		t.Errorf("values[env] = %q, want production", got.Values["env"])
	}
	if got.Values["version"] != "1.2.3" {
		t.Errorf("values[version] = %q, want 1.2.3", got.Values["version"])
	}
}

func TestFlagsCaddyfile_EmptyBlock(t *testing.T) {
	raw, err := parse(t, "")
	if err != nil {
		t.Fatalf("ParseCaddyfile: %v", err)
	}
	var got struct {
		Values map[string]string `json:"values"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Values) != 0 {
		t.Errorf("values = %v, want empty map", got.Values)
	}
}

func TestFlagsCaddyfile_Errors(t *testing.T) {
	cases := map[string]string{
		"missing value": "\t\tkey",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parse(t, body); err == nil {
				t.Errorf("expected error for %q", name)
			}
		})
	}
}
