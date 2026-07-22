// Package caddyfile registers the git source Caddyfile parser.
package caddyfile

import (
	"encoding/json"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	xtc "github.com/infogulch/xtemplate/caddy"

	_ "github.com/infogulch/xtemplate/sources/git"
)

func init() {
	caddy.RegisterModule(gitCaddyfile{})
}

type gitCaddyfile struct{}

func (gitCaddyfile) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "xtemplate.source.git",
		New: func() caddy.Module { return new(gitCaddyfile) },
	}
}

var _ xtc.CaddyfileBlockParser = (*gitCaddyfile)(nil)

func (gitCaddyfile) ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error) {
	var (
		repo, ref, interval, path string
	)
	for h.NextBlock(1) {
		switch h.Val() {
		case "repo":
			if !h.AllArgs(&repo) {
				return nil, h.ArgErr()
			}
		case "ref":
			if !h.AllArgs(&ref) {
				return nil, h.ArgErr()
			}
		case "interval":
			if !h.AllArgs(&interval) {
				return nil, h.ArgErr()
			}
		case "path":
			if !h.AllArgs(&path) {
				return nil, h.ArgErr()
			}
		default:
			return nil, h.Errf("unknown git source option '%s'", h.Val())
		}
	}
	return json.Marshal(struct {
		Repo     string `json:"repo,omitempty"`
		Ref      string `json:"ref,omitempty"`
		Interval string `json:"interval,omitempty"`
		Path     string `json:"path,omitempty"`
	}{repo, ref, interval, path})
}
