package register

import (
	"fmt"
	"html/template"
	"io/fs"

	"github.com/infogulch/xtemplate/internal"
)

func RegisterFuncMap(name string, funcs template.FuncMap) {
	if existing, ok := internal.RegisteredFuncMaps[name]; ok {
		panic(fmt.Sprintf("funcmap named '%s' already registered as '%+v'", name, existing))
	}
	internal.RegisteredFuncMaps[name] = funcs
}

func RegisterFS(name string, fs fs.FS) {
	if existing, ok := internal.RegisteredFS[name]; ok {
		panic(fmt.Sprintf("fs named '%s' already registered as '%+v'", name, existing))
	}
	internal.RegisteredFS[name] = fs
}
