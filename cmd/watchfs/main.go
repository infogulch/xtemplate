// The default xtemplate CLI package. Watches the templates directory and
// reloads the server when they change. To customize, copy this file to a new
// package and pass config overrides to watchfs.Main.
package main

import (
	"github.com/infogulch/xtemplate/app/watchfs"

	_ "github.com/infogulch/xtemplate/providers/flags"
	_ "github.com/infogulch/xtemplate/providers/fs"
	_ "github.com/infogulch/xtemplate/providers/nats"
	_ "github.com/infogulch/xtemplate/providers/sql"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func main() {
	watchfs.Main()
}
