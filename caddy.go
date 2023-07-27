package xtemplates

import (
	"html/template"
	"io/fs"

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
				if !h.Args(&t.Root) {
					return nil, h.ArgErr()
				}
			case "fs_module":
				if !h.Args((*string)(&t.FSModule)) {
					return nil, h.ArgErr()
				}
			case "func_modules":
				for _, arg := range h.RemainingArgs() {
					t.FuncModules = append(t.FuncModules, (caddy.ModuleID)(arg))
				}
			}
		}
	}
	return t, nil
}

// CustomFSProvider is the interface for registering custom file system for loading templates.
type CustomFSProvider interface {
	CustomTemplateFS() fs.FS
}

// CustomFunctionsProvider is the interface for registering custom template functions.
type CustomFunctionsProvider interface {
	// CustomTemplateFunctions should return the mapping from custom function names to implementations.
	CustomTemplateFunctions() template.FuncMap
}
