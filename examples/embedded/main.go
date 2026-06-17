// Embedded single-binary example: the templates dir is compiled into the binary
// via //go:embed, so this binary serves its templates with no templates dir on
// disk at runtime. Run it from anywhere. CLI flags (e.g. --listen) still work.
package main

import (
	"embed"
	"io/fs"

	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/app"

	"github.com/spf13/afero"
)

//go:embed all:templates
var templatesFS embed.FS

func main() {
	sub, err := fs.Sub(templatesFS, "templates")
	if err != nil {
		panic(err)
	}
	app.Main(xtemplate.WithTemplateFS(afero.FromIOFS{FS: sub}))
}
