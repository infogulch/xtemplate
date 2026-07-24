package xtemplate_caddy

import (
	"encoding/json"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

func init() {
	caddy.RegisterModule(osCaddyfile{})
}

// osCaddyfile parses `source os { path <dir> }` blocks.
type osCaddyfile struct{}

func (osCaddyfile) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "xtemplate.source.os",
		New: func() caddy.Module { return new(osCaddyfile) },
	}
}

var _ CaddyfileBlockParser = (*osCaddyfile)(nil)

func (osCaddyfile) ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error) {
	var path string
	for h.NextBlock(1) {
		switch h.Val() {
		case "path":
			if !h.AllArgs(&path) {
				return nil, h.ArgErr()
			}
		default:
			return nil, h.Errf("unknown os source option '%s'", h.Val())
		}
	}
	return json.Marshal(struct {
		Path string `json:"path,omitempty"`
	}{path})
}
