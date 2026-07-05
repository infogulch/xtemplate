package caddyfile_test

import (
	"encoding/json"
	"testing"

	caddy "github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"

	xtc "github.com/infogulch/xtemplate/caddy"
	_ "github.com/infogulch/xtemplate/providers/dotfs/caddyfile"
)

func parse(t *testing.T, body string) (json.RawMessage, error) {
	t.Helper()
	input := "outer {\n\tprovider fs FS {\n" + body + "\n\t}\n}"
	h := httpcaddyfile.Helper{}.WithDispenser(caddyfile.NewTestDispenser(input))
	h.Next()       // "outer"
	h.NextBlock(0) // → "provider"
	h.NextArg()    // "fs"
	h.NextArg()    // "FS"
	mi, err := caddy.GetModule("xtemplate.providers.fs")
	if err != nil {
		t.Fatalf("GetModule: %v", err)
	}
	return mi.New().(xtc.CaddyfileProvider).ParseCaddyfile(h)
}

func TestFsCaddyfile_HappyPath(t *testing.T) {
	raw, err := parse(t, "\t\tpath /srv/data")
	if err != nil {
		t.Fatalf("ParseCaddyfile: %v", err)
	}
	var got struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Path != "/srv/data" {
		t.Errorf("path = %q, want /srv/data", got.Path)
	}
}

func TestFsCaddyfile_EmptyBlock(t *testing.T) {
	raw, err := parse(t, "")
	if err != nil {
		t.Fatalf("ParseCaddyfile: %v", err)
	}
	var got struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Path != "" {
		t.Errorf("path = %q, want empty", got.Path)
	}
}

func TestFsCaddyfile_Errors(t *testing.T) {
	cases := map[string]string{
		"unknown key":      "\t\tbogus val",
		"missing path arg": "\t\tpath",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parse(t, body); err == nil {
				t.Errorf("expected error for %q", name)
			}
		})
	}
}
