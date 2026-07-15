// Basic xtemplate CLI package. To customize, copy this file to a new unique
// package to configured your db drivers and provide config overrides.
package main

import (
	"github.com/infogulch/xtemplate/app"

	_ "github.com/infogulch/xtemplate/providers/dotflags"
	_ "github.com/infogulch/xtemplate/providers/dotfs"
	_ "github.com/infogulch/xtemplate/providers/dotnats"
	_ "github.com/infogulch/xtemplate/providers/dotsmtp"
	_ "github.com/infogulch/xtemplate/providers/dotsql"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func main() {
	app.Main()
}
