// Default CLI package. To customize, copy this file to a new unique package and
// import dbs and provide config overrides.
package main

import (
	"github.com/infogulch/xtemplate/app"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

func main() {
	app.Main()
}
