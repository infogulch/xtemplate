// Default xtemplate CLI: providers, watchfs, git; default controller is watchfs.
package main

import (
	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/app"

	_ "github.com/infogulch/xtemplate/providers/dotbus"
	_ "github.com/infogulch/xtemplate/providers/dotflags"
	_ "github.com/infogulch/xtemplate/providers/dotfs"
	_ "github.com/infogulch/xtemplate/providers/dotnats"
	_ "github.com/infogulch/xtemplate/providers/dotsmtp"
	_ "github.com/infogulch/xtemplate/providers/dotsql"

	_ "github.com/infogulch/xtemplate/controllers/git"
	_ "github.com/infogulch/xtemplate/controllers/watchfs"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func main() {
	xtemplate.DefaultControllerType = "watchfs"
	app.Main()
}
