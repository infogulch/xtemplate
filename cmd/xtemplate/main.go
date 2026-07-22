// The default xtemplate CLI. Blank-imports providers and optional sources
// (watchfs, git). Default --source-type is watchfs (override with the flag or
// Docker ldflag defaultSourceType=os).
package main

import (
	"github.com/infogulch/xtemplate/app"

	_ "github.com/infogulch/xtemplate/providers/dotbus"
	_ "github.com/infogulch/xtemplate/providers/dotflags"
	_ "github.com/infogulch/xtemplate/providers/dotfs"
	_ "github.com/infogulch/xtemplate/providers/dotnats"
	_ "github.com/infogulch/xtemplate/providers/dotsmtp"
	_ "github.com/infogulch/xtemplate/providers/dotsql"

	_ "github.com/infogulch/xtemplate/sources/git"
	_ "github.com/infogulch/xtemplate/sources/watchfs"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func main() {
	app.Main()
}
