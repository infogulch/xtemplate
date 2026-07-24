// Package standard links the default set of dot-provider Caddyfile parsers
// (sql, fs, flags, nats, smtp, bus), optional source Caddyfile parsers
// (watchfs, git), the pure-Go sqlite3 database/sql driver, and the
// xtemplate caddy module (including built-in source os) in a single opt-in import.
//
// Usage:
//
//	xcaddy build --with github.com/infogulch/xtemplate/caddy/standard
//
// The blank imports also pull in the providers' and sources' xtemplate
// constructors, so this package doubles as the one-stop link for caddy JSON
// users who want the default set without per-package --with lines.
package standard

import (
	_ "github.com/infogulch/xtemplate/caddy"
	_ "github.com/infogulch/xtemplate/providers/dotbus/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/dotflags/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/dotfs/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/dotnats/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/dotsmtp/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/dotsql/caddyfile"
	_ "github.com/infogulch/xtemplate/sources/git/caddyfile"
	_ "github.com/infogulch/xtemplate/sources/watchfs/caddyfile"

	_ "github.com/ncruces/go-sqlite3/driver"
)
