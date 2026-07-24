// Package caddyfile registers the flags dot-provider for Caddyfile use.
// Blank-import this package to enable `provider flags <field> { }` blocks.
// Each line in the block is a key-value pair: `key value`.
package caddyfile

import (
	"encoding/json"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	xtc "github.com/infogulch/xtemplate/caddy"
	_ "github.com/infogulch/xtemplate/providers/dotflags"
)

type flagsCaddyfile struct{}

func init() { caddy.RegisterModule(flagsCaddyfile{}) }

func (flagsCaddyfile) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "xtemplate.providers.flags",
		New: func() caddy.Module { return new(flagsCaddyfile) },
	}
}

var _ xtc.CaddyfileBlockParser = (*flagsCaddyfile)(nil)

func (flagsCaddyfile) ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error) {
	values := map[string]string{}
	for h.NextBlock(1) {
		key := h.Val()
		var val string
		if !h.AllArgs(&val) {
			return nil, h.Errf("flag %q requires exactly one value", key)
		}
		values[key] = val
	}
	return json.Marshal(struct {
		Values map[string]string `json:"values"`
	}{values})
}
