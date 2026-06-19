// The default xtemplate CLI package. Watches the templates directory and
// reloads the server when they change. To customize, copy this file to a new
// package and pass config overrides to watchfs.Main.
package main

import (
	"github.com/infogulch/xtemplate/app/watchfs"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func main() {
	watchfs.Main()
}
