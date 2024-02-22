// Default CLI package. To customize, copy this file to a new unique package and
// import dbs and provide config overrides.
package main

import "github.com/infogulch/xtemplate"

func main() {
	xtemplate.Main( /* Main accepts some configuration overrides, see ../config.go */ )
}
