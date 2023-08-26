package xtemplate

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
		ID:  "http.handlers.xtemplate",
		New: func() caddy.Module { return new(Templates) },
	}
}

func init() {
	httpcaddyfile.RegisterHandlerDirective("xtemplate", parseCaddyfile)
}

// parseCaddyfile sets up the handler from Caddyfile tokens. Syntax:
//
//	xtemplate [<matcher>] {
//	    database <driver> <connstr>
//	    delimiters <open_delim> <close_delim>
//	    root <path>
//	}
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	t := new(Templates)
	t.Config = make(map[string]string)
	for h.Next() {
		for h.NextBlock(0) {
			switch h.Val() {
			case "database":
				for nesting := h.Nesting(); h.NextBlock(nesting); {
					switch h.Val() {
					case "driver":
						if !h.Args(&t.Database.Driver) {
							return nil, h.ArgErr()
						}
					case "connstr":
						if !h.Args(&t.Database.Connstr) {
							return nil, h.ArgErr()
						}
					}
				}
			case "delimiters":
				t.Delimiters = h.RemainingArgs()
				if len(t.Delimiters) != 2 {
					return nil, h.ArgErr()
				}
			case "template_root":
				if !h.Args(&t.TemplateRoot) {
					return nil, h.ArgErr()
				}
			case "context_root":
				if !h.Args(&t.ContextRoot) {
					return nil, h.ArgErr()
				}
			case "config":
				for nesting := h.Nesting(); h.NextBlock(nesting); {
					var key, val string
					key = h.Val()
					if _, ok := t.Config[key]; ok {
						return nil, h.Errf("Config key '%s' repeated", key)
					}
					if !h.Args(&val) {
						return nil, h.ArgErr()
					}
					t.Config[key] = val
				}
			case "funcs_modules":
				t.FuncsModules = h.RemainingArgs()
			}
		}
	}
	return t, nil
}
