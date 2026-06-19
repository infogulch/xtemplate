// Git-backed xtemplate CLI package. Serves templates from a git repository,
// polling for changes and hot-reloading. To customize, copy this file to a new
// package and pass config overrides to appgit.Main.
package main

import (
	"github.com/infogulch/xtemplate/app/git"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func main() {
	git.Main()
}
