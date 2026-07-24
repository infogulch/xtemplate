package xtemplate_caddy

import (
	"strconv"

	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/infogulch/xtemplate"
)

func init() {
	httpcaddyfile.RegisterHandlerDirective("xtemplate", parseCaddyfile)
}

// parseCaddyfile sets up the handler from Caddyfile tokens.
// Default without a source block is os path templates. Legacy
// templates_dir / templates_path / watch_template_path hard-reject.
//
// REMOVE BEFORE 1.0: temporary hard-rejects for renamed Caddy knobs.
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	t := &XTemplateModule{
		Config: *xtemplate.New(),
	}

	for h.Next() {
		for h.NextBlock(0) {
			switch h.Val() {
			// REMOVE BEFORE 1.0
			case "templates_dir", "templates_path":
				return nil, h.Errf("templates_dir is no longer supported; use: source os { path <dir> } (or source watchfs { path <dir> … } if they need reload)")
			// REMOVE BEFORE 1.0
			case "watch_template_path":
				return nil, h.Errf("watch_template_path is no longer supported; use: source watchfs { path … } (default is os / no watch)")
			case "template_extension":
				if !h.AllArgs(&t.TemplateExtension) {
					return nil, h.ArgErr()
				}
			case "delimiters":
				if !h.AllArgs(&t.LDelim, &t.RDelim) {
					return nil, h.ArgErr()
				}
			case "minify":
				b, err := parseBoolArg(h)
				if err != nil {
					return nil, err
				}
				t.Minify = &b
			case "precompress":
				args := h.RemainingArgs()
				if len(args) == 0 {
					return nil, h.ArgErr()
				}
				for _, enc := range args {
					switch enc {
					case "gzip", "zstd", "br":
						t.Precompress = append(t.Precompress, enc)
					default:
						return nil, h.Errf("unknown precompress encoding %q; want gzip, zstd, or br", enc)
					}
				}
			case "crossorigin":
				if err := parseCrossOrigin(h, &t.CrossOrigin); err != nil {
					return nil, err
				}
			case "provider":
				if err := parseProviderBlock(h, t); err != nil {
					return nil, err
				}
			case "source":
				if err := parseSourceBlock(h, t); err != nil {
					return nil, err
				}
			default:
				return nil, h.Errf("unknown config option")
			}
		}
	}
	return t, nil
}

// parseCrossOrigin parses the `crossorigin` block into a CrossOriginConfig.
func parseCrossOrigin(h httpcaddyfile.Helper, cors *xtemplate.CrossOriginConfig) error {
	for h.NextBlock(1) {
		switch h.Val() {
		case "disabled":
			b, err := parseBoolArg(h)
			if err != nil {
				return err
			}
			cors.Disabled = b
		case "trusted_origins":
			args := h.RemainingArgs()
			if len(args) == 0 {
				return h.ArgErr()
			}
			cors.TrustedOrigins = append(cors.TrustedOrigins, args...)
		case "insecure_bypass_patterns":
			args := h.RemainingArgs()
			if len(args) == 0 {
				return h.ArgErr()
			}
			cors.InsecureBypassPatterns = append(cors.InsecureBypassPatterns, args...)
		default:
			return h.Errf("unknown crossorigin option '%s'", h.Val())
		}
	}
	return nil
}

// parseBoolArg reads a single boolean argument from the current directive.
func parseBoolArg(h httpcaddyfile.Helper) (bool, error) {
	var boolstr string
	if !h.AllArgs(&boolstr) {
		return false, h.ArgErr()
	}
	b, err := strconv.ParseBool(boolstr)
	if err != nil {
		return false, h.Errf("arg must be bool, got `%s`: %s", boolstr, err)
	}
	return b, nil
}
