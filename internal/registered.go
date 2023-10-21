package internal

import (
	"html/template"
	"io/fs"
)

var RegisteredFuncMaps map[string]template.FuncMap = make(map[string]template.FuncMap)

var RegisteredFS map[string]fs.FS = make(map[string]fs.FS)
