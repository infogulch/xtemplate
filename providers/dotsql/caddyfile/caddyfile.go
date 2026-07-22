// Package caddyfile registers the sql dot-provider for Caddyfile use.
// Blank-import this package to enable `provider sql <field> { }` blocks.
package caddyfile

import (
	"encoding/json"
	"strconv"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	xtc "github.com/infogulch/xtemplate/caddy"
	_ "github.com/infogulch/xtemplate/providers/dotsql"
)

type sqlCaddyfile struct{}

func init() { caddy.RegisterModule(sqlCaddyfile{}) }

func (sqlCaddyfile) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "xtemplate.providers.sql",
		New: func() caddy.Module { return new(sqlCaddyfile) },
	}
}

var _ xtc.CaddyfileBlockParser = (*sqlCaddyfile)(nil)

func (sqlCaddyfile) ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error) {
	var driver, connstr string
	var maxOpen int
	for h.NextBlock(1) {
		switch h.Val() {
		case "driver":
			if !h.AllArgs(&driver) {
				return nil, h.ArgErr()
			}
		case "connstr":
			if !h.AllArgs(&connstr) {
				return nil, h.ArgErr()
			}
		case "max_open_conns":
			var s string
			if !h.AllArgs(&s) {
				return nil, h.ArgErr()
			}
			n, err := strconv.Atoi(s)
			if err != nil {
				return nil, h.Errf("max_open_conns must be int: %v", err)
			}
			maxOpen = n
		default:
			return nil, h.Errf("unknown sql provider option '%s'", h.Val())
		}
	}
	return json.Marshal(struct {
		Driver       string `json:"driver"`
		Connstr      string `json:"connstr"`
		MaxOpenConns int    `json:"max_open_conns,omitempty"`
	}{driver, connstr, maxOpen})
}
