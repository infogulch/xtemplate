// Package standard links the default Caddyfile parsers (providers + controllers),
// the pure-Go sqlite3 driver, and the xtemplate caddy module. Default controller
// type is watchfs.
//
//	xcaddy build --with github.com/infogulch/xtemplate/caddy/standard
package standard

import (
	"github.com/infogulch/xtemplate"

	_ "github.com/infogulch/xtemplate/caddy"

	_ "github.com/infogulch/xtemplate/controllers/git/caddyfile"
	_ "github.com/infogulch/xtemplate/controllers/watchfs/caddyfile"

	_ "github.com/infogulch/xtemplate/providers/dotbus/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/dotflags/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/dotfs/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/dotnats/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/dotsmtp/caddyfile"
	_ "github.com/infogulch/xtemplate/providers/dotsql/caddyfile"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func init() {
	xtemplate.DefaultControllerType = "watchfs"
}
