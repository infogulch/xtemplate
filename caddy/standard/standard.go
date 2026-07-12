// Package standard links the default set of dot-provider Caddyfile parsers
// (sql, fs, flags, nats), the pure-Go sqlite3 database/sql driver, and the
// xtemplate caddy module in a single opt-in import.
//
// Usage:
//
//	xcaddy build --with github.com/infogulch/xtemplate/caddy/standard
//
// The blank imports also pull in the providers' xtemplate constructors, so this
// package doubles as the one-stop link for caddy JSON users who want the
// default set without per-provider --with lines.
package standard

import (
	_ "github.com/infogulch/xtemplate/caddy"
	_ "github.com/infogulch/xtemplate/providers/dotflags/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/dotfs/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/dotnats/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/dotsql/caddyfile"

	_ "github.com/ncruces/go-sqlite3/driver"
)
