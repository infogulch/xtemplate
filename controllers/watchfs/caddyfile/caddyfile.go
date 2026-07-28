// Package caddyfile registers the watchfs controller Caddyfile parser.
package caddyfile

import (
	"encoding/json"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	xtc "github.com/infogulch/xtemplate/caddy"

	_ "github.com/infogulch/xtemplate/controllers/watchfs"
)

func init() {
	caddy.RegisterModule(watchfsCaddyfile{})
}

type watchfsCaddyfile struct{}

func (watchfsCaddyfile) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "xtemplate.controller.watchfs",
		New: func() caddy.Module { return new(watchfsCaddyfile) },
	}
}

var _ xtc.CaddyfileBlockParser = (*watchfsCaddyfile)(nil)

func (watchfsCaddyfile) ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error) {
	var (
		path     string
		debounce string
		watch    []string
	)
	for h.NextBlock(1) {
		switch h.Val() {
		case "path":
			if !h.AllArgs(&path) {
				return nil, h.ArgErr()
			}
		case "debounce":
			if !h.AllArgs(&debounce) {
				return nil, h.ArgErr()
			}
		case "watch_dirs", "watch":
			args := h.RemainingArgs()
			if len(args) == 0 {
				return nil, h.ArgErr()
			}
			watch = append(watch, args...)
		default:
			return nil, h.Errf("unknown watchfs controller option '%s'", h.Val())
		}
	}
	return json.Marshal(struct {
		Path      string   `json:"path,omitempty"`
		Debounce  string   `json:"debounce,omitempty"`
		WatchDirs []string `json:"watch_dirs,omitempty"`
	}{path, debounce, watch})
}
