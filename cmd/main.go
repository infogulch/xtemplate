// Basic xtemplate CLI package. To customize, copy this file to a new unique
// package to configured your db drivers and provide config overrides.
package main

import (
	"github.com/infogulch/xtemplate/app"

	_ "github.com/infogulch/xtemplate/providers/flags"
	_ "github.com/infogulch/xtemplate/providers/fs"
	_ "github.com/infogulch/xtemplate/providers/nats"
	_ "github.com/infogulch/xtemplate/providers/sql"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func main() {
	app.Main()
}
