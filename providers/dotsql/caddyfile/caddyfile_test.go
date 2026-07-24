package caddyfile_test

import (
	"encoding/json"
	"testing"

	caddy "github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"

	xtc "github.com/infogulch/xtemplate/caddy"
	_ "github.com/infogulch/xtemplate/providers/dotsql/caddyfile"
)

// parse simulates the nesting depth that ParseCaddyfile receives at the real
// call site: cursor is after the field name at depth 1, next token is the
// opening brace of the provider block.
func parse(t *testing.T, body string) (json.RawMessage, error) {
	t.Helper()
	input := "outer {\n\tprovider sql DB {\n" + body + "\n\t}\n}"
	h := httpcaddyfile.Helper{}.WithDispenser(caddyfile.NewTestDispenser(input))
	h.Next()       // "outer" depth 0
	h.NextBlock(0) // → "provider" depth 1
	h.NextArg()    // "sql"
	h.NextArg()    // "DB" - cursor now matches real call site
	mi, err := caddy.GetModule("xtemplate.providers.sql")
	if err != nil {
		t.Fatalf("GetModule: %v", err)
	}
	p := mi.New().(xtc.CaddyfileBlockParser)
	return p.ParseCaddyfile(h)
}

func TestSqlCaddyfile_HappyPath(t *testing.T) {
	raw, err := parse(t, "\t\tdriver sqlite3\n\t\tconnstr file:test.db\n\t\tmax_open_conns 5")
	if err != nil {
		t.Fatalf("ParseCaddyfile: %v", err)
	}
	var got struct {
		Driver       string `json:"driver"`
		Connstr      string `json:"connstr"`
		MaxOpenConns int    `json:"max_open_conns"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Driver != "sqlite3" {
		t.Errorf("driver = %q, want sqlite3", got.Driver)
	}
	if got.Connstr != "file:test.db" {
		t.Errorf("connstr = %q, want file:test.db", got.Connstr)
	}
	if got.MaxOpenConns != 5 {
		t.Errorf("max_open_conns = %d, want 5", got.MaxOpenConns)
	}
}

func TestSqlCaddyfile_OmitsMaxOpenConns(t *testing.T) {
	raw, err := parse(t, "\t\tdriver postgres\n\t\tconnstr host=localhost")
	if err != nil {
		t.Fatalf("ParseCaddyfile: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["max_open_conns"]; ok {
		t.Error("max_open_conns should be omitted when zero")
	}
}

func TestSqlCaddyfile_Errors(t *testing.T) {
	cases := map[string]string{
		"unknown key":            "\t\tbogus val",
		"non-int max_open_conns": "\t\tmax_open_conns notanint",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parse(t, body); err == nil {
				t.Errorf("expected error for %q", name)
			}
		})
	}
}
