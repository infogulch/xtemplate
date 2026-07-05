// This file bundles the default dot-provider constructors into the xtemplate
// Caddy module so that JSON configs using the built-in provider types (sql, fs,
// flags, nats) work out of the box. The blank imports register each provider's
// xtemplate.Register constructor via their init() functions.
package xtemplate_caddy

import (
	_ "github.com/infogulch/xtemplate/providers/dotflags"
	_ "github.com/infogulch/xtemplate/providers/dotfs"
	_ "github.com/infogulch/xtemplate/providers/dotnats"
	_ "github.com/infogulch/xtemplate/providers/dotsql"
)
