package xtemplates

import (
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	caddy.RegisterModule(Templates{})
}

// CaddyModule returns the Caddy module information.
func (Templates) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.xtemplates",
		New: func() caddy.Module { return new(Templates) },
	}
}

func init() {
	httpcaddyfile.RegisterHandlerDirective("xtemplates", parseCaddyfile)
}

// parseCaddyfile sets up the handler from Caddyfile tokens. Syntax:
//
//	xtemplates [<matcher>] {
//	    database <driver> <connstr>
//	    delimiters <open_delim> <close_delim>
//	    root <path>
//	}
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	t := new(Templates)
	for h.Next() {
		for h.NextBlock(0) {
			switch h.Val() {
			case "database":
				t.Database = h.RemainingArgs()
				if len(t.Database) != 2 {
					return nil, h.ArgErr()
				}
			case "delimiters":
				t.Delimiters = h.RemainingArgs()
				if len(t.Delimiters) != 2 {
					return nil, h.ArgErr()
				}
			case "root":
				if !h.Args(&t.FileRoot) {
					return nil, h.ArgErr()
				}
			}
		}
	}
	return t, nil
}
