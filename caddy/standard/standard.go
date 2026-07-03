// Package standard links the default set of dot-provider Caddyfile parsers
// (sql, fs, flags) in a single opt-in import.
//
// Usage:
//
//	xcaddy build \
//	  --with github.com/infogulch/xtemplate/caddy \
//	  --with github.com/infogulch/xtemplate/caddy/standard
//
// The blank imports here also pull in the providers' constructors, so this
// package doubles as the one-stop link for caddy JSON users who want the
// default set without per-provider --with lines.
package standard

import (
	_ "github.com/infogulch/xtemplate/providers/flags/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/fs/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/sql/caddyfile"
)
