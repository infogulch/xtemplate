// Package caddyfile registers the nats dot-provider for Caddyfile use.
// Blank-import this package to enable `provider nats <field> { }` blocks.
//
// Covered: in_process_server { dont_listen } and conn_options { url }.
// JetStream options are Go-API-only (no JSON representation) and are not covered.
package caddyfile

import (
	"encoding/json"
	"strconv"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	xtc "github.com/infogulch/xtemplate/caddy"
	_ "github.com/infogulch/xtemplate/providers/dotnats"
)

type natsCaddyfile struct{}

func init() { caddy.RegisterModule(natsCaddyfile{}) }

func (natsCaddyfile) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "xtemplate.providers.nats",
		New: func() caddy.Module { return new(natsCaddyfile) },
	}
}

var _ xtc.CaddyfileProvider = (*natsCaddyfile)(nil)

func (natsCaddyfile) ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error) {
	// Local structs mirror only the curated JSON subset of NatsConfig /
	// server.Options / natsgo.Options - no import of those packages needed.
	type inProcessServer struct {
		DontListen bool `json:"dont_listen,omitempty"`
	}
	type connOptions struct {
		// natsgo.Options.Url has no json tag, so the key is the field name "Url".
		Url string `json:"Url,omitempty"`
	}
	type natsConfig struct {
		InProcessServer *inProcessServer `json:"in_process_server_options,omitempty"`
		ConnOptions     *connOptions     `json:"conn_options,omitempty"`
	}
	type result struct {
		NatsConfig *natsConfig `json:"nats_config,omitempty"`
	}

	cfg := &natsConfig{}
	for h.NextBlock(1) {
		switch h.Val() {
		case "in_process_server":
			cfg.InProcessServer = &inProcessServer{}
			for h.NextBlock(2) {
				switch h.Val() {
				case "dont_listen":
					var s string
					if !h.AllArgs(&s) {
						return nil, h.ArgErr()
					}
					b, err := strconv.ParseBool(s)
					if err != nil {
						return nil, h.Errf("dont_listen must be bool: %v", err)
					}
					cfg.InProcessServer.DontListen = b
				default:
					return nil, h.Errf("unknown in_process_server option '%s'", h.Val())
				}
			}
		case "conn_options":
			cfg.ConnOptions = &connOptions{}
			for h.NextBlock(2) {
				switch h.Val() {
				case "url":
					if !h.AllArgs(&cfg.ConnOptions.Url) {
						return nil, h.ArgErr()
					}
				default:
					return nil, h.Errf("unknown conn_options option '%s'", h.Val())
				}
			}
		default:
			return nil, h.Errf("unknown nats provider option '%s'", h.Val())
		}
	}
	return json.Marshal(result{cfg})
}
