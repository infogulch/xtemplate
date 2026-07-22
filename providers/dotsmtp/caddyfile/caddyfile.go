// Package caddyfile registers the smtp dot-provider for Caddyfile use.
// Blank-import this package to enable `provider smtp <field> { }` blocks.
//
// Covered: host, port, username, password, auth, tls, from, max_recipients,
// max_message_bytes, send_timeout. Only the curated JSON subset of
// DotSMTPConfig is mirrored here — no import of go-mail is needed.
package caddyfile

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	xtc "github.com/infogulch/xtemplate/caddy"
	_ "github.com/infogulch/xtemplate/providers/dotsmtp"
)

type smtpCaddyfile struct{}

func init() { caddy.RegisterModule(smtpCaddyfile{}) }

func (smtpCaddyfile) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "xtemplate.providers.smtp",
		New: func() caddy.Module { return new(smtpCaddyfile) },
	}
}

var _ xtc.CaddyfileBlockParser = (*smtpCaddyfile)(nil)

func (smtpCaddyfile) ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error) {
	// Local struct mirrors only the curated JSON subset of DotSMTPConfig.
	// SendTimeout is emitted as a duration string (e.g. "45s"); the target
	// field is xtemplate.Duration.
	type result struct {
		Host            string `json:"host,omitempty"`
		Port            int    `json:"port,omitempty"`
		Username        string `json:"username,omitempty"`
		Password        string `json:"password,omitempty"`
		Auth            string `json:"auth,omitempty"`
		TLS             string `json:"tls,omitempty"`
		Helo            string `json:"helo,omitempty"`
		From            string `json:"from,omitempty"`
		MaxRecipients   int    `json:"max_recipients,omitempty"`
		MaxMessageBytes int64  `json:"max_message_bytes,omitempty"`
		SendTimeout     string `json:"send_timeout,omitempty"`
	}

	cfg := &result{}
	for h.NextBlock(1) {
		key := h.Val()
		switch key {
		case "host", "username", "password", "auth", "tls", "helo", "from":
			var s string
			if !h.AllArgs(&s) {
				return nil, h.ArgErr()
			}
			switch key {
			case "host":
				cfg.Host = s
			case "username":
				cfg.Username = s
			case "password":
				cfg.Password = s
			case "auth":
				cfg.Auth = s
			case "tls":
				cfg.TLS = s
			case "helo":
				cfg.Helo = s
			case "from":
				cfg.From = s
			}
		case "port", "max_recipients":
			var s string
			if !h.AllArgs(&s) {
				return nil, h.ArgErr()
			}
			n, err := strconv.Atoi(s)
			if err != nil {
				return nil, h.Errf("%s must be an integer: %v", key, err)
			}
			if key == "port" {
				cfg.Port = n
			} else {
				cfg.MaxRecipients = n
			}
		case "max_message_bytes":
			var s string
			if !h.AllArgs(&s) {
				return nil, h.ArgErr()
			}
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return nil, h.Errf("max_message_bytes must be an integer: %v", err)
			}
			cfg.MaxMessageBytes = n
		case "send_timeout":
			var s string
			if !h.AllArgs(&s) {
				return nil, h.ArgErr()
			}
			if _, err := time.ParseDuration(s); err != nil {
				return nil, h.Errf("send_timeout must be a duration (e.g. 30s): %v", err)
			}
			cfg.SendTimeout = s
		default:
			return nil, h.Errf("unknown smtp provider option '%s'", key)
		}
	}

	if cfg.Host == "" {
		return nil, h.Errf("smtp provider requires a 'host' key")
	}
	if cfg.From == "" {
		return nil, h.Errf("smtp provider requires a 'from' key")
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("smtp: marshal config: %w", err)
	}
	return out, nil
}
