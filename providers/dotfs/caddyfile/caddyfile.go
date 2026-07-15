// Package caddyfile registers the fs dot-provider for Caddyfile use.
// Blank-import this package to enable `provider fs <field> { }` blocks.
package caddyfile

import (
	"encoding/json"
	"strconv"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	xtc "github.com/infogulch/xtemplate/caddy"
	_ "github.com/infogulch/xtemplate/providers/dotfs"
)

type fsCaddyfile struct{}

func init() { caddy.RegisterModule(fsCaddyfile{}) }

func (fsCaddyfile) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "xtemplate.providers.fs",
		New: func() caddy.Module { return new(fsCaddyfile) },
	}
}

var _ xtc.CaddyfileProvider = (*fsCaddyfile)(nil)

func (fsCaddyfile) ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error) {
	var path string
	var writable bool
	for h.NextBlock(1) {
		switch h.Val() {
		case "path":
			if !h.AllArgs(&path) {
				return nil, h.ArgErr()
			}
		case "writable":
			var s string
			if !h.AllArgs(&s) {
				return nil, h.ArgErr()
			}
			b, err := strconv.ParseBool(s)
			if err != nil {
				return nil, h.Errf("writable must be a boolean: %v", err)
			}
			writable = b
		default:
			return nil, h.Errf("unknown fs provider option '%s'", h.Val())
		}
	}
	return json.Marshal(struct {
		Path     string `json:"path"`
		Writable bool   `json:"writable,omitempty"`
	}{path, writable})
}
