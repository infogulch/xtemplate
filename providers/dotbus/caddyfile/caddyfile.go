// Package caddyfile registers the bus dot-provider for Caddyfile use.
// Blank-import this package to enable `provider bus <field> { }` blocks.
//
// Covered: buffer.
package caddyfile

import (
	"encoding/json"
	"strconv"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	xtc "github.com/infogulch/xtemplate/caddy"
	_ "github.com/infogulch/xtemplate/providers/dotbus"
)

type busCaddyfile struct{}

func init() { caddy.RegisterModule(busCaddyfile{}) }

func (busCaddyfile) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "xtemplate.providers.bus",
		New: func() caddy.Module { return new(busCaddyfile) },
	}
}

var _ xtc.CaddyfileProvider = (*busCaddyfile)(nil)

func (busCaddyfile) ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error) {
	type result struct {
		Buffer int `json:"buffer,omitempty"`
	}
	cfg := &result{}
	for h.NextBlock(1) {
		switch h.Val() {
		case "buffer":
			var s string
			if !h.AllArgs(&s) {
				return nil, h.ArgErr()
			}
			n, err := strconv.Atoi(s)
			if err != nil {
				return nil, h.Errf("buffer must be an integer: %v", err)
			}
			if n < 0 {
				return nil, h.Errf("buffer must be >= 0")
			}
			cfg.Buffer = n
		default:
			return nil, h.Errf("unknown bus provider option '%s'", h.Val())
		}
	}
	return json.Marshal(cfg)
}
