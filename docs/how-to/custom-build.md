# Custom build

Ship a binary with the drivers, providers, controllers, and defaults you need.

## Stock entries

| Build | Path | Notes |
|---|---|---|
| GitHub release / default CLI | `./cmd/xtemplate` | Providers + watchfs + git; default controller `watchfs` |
| Docker image | `infogulch/xtemplate` | Same entry; ldflag sets listen `:80` |

## App package

| Import | Role |
|---|---|
| `github.com/infogulch/xtemplate/app` | CLI load + serve (`app.Main`) |

Optional controllers (blank-import to register):

| Import | Type string |
|---|---|
| `github.com/infogulch/xtemplate/controllers/watchfs` | `watchfs` |
| `github.com/infogulch/xtemplate/controllers/git` | `git` |

Core `DefaultControllerType` is `"os"`. Set it in `main` if you want another default.

## Minimal main

Default `cmd/xtemplate/main.go` shape:

```go
package main

import (
	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/app"

	_ "github.com/infogulch/xtemplate/providers/dotsql"
	// … other providers …
	_ "github.com/infogulch/xtemplate/controllers/git"
	_ "github.com/infogulch/xtemplate/controllers/watchfs"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func main() {
	xtemplate.DefaultControllerType = "watchfs"
	app.Main()
}
```

Pass overrides:

```go
app.Main(
	xtemplate.WithFuncMaps(myFuncs),
	xtemplate.WithProvider(func() xtemplate.Provider { return myProvider{} }),
)
```

## Drivers and providers

Blank-import `database/sql` drivers and provider packages so `RegisterProvider` runs in `init`. See [Create a provider](create-a-provider.md).

## Embed templates

```go
//go:embed templates/*
var templates embed.FS

func main() {
	fs := afero.FromIOFS{FS: templates}
	app.Main(xtemplate.WithTemplateFS(fs))
}
```

## Caddy custom build

```shell
xcaddy build \
  --with github.com/infogulch/xtemplate/caddy/standard
```

Or pick individual `providers/*/caddyfile` and `controllers/*/caddyfile` modules.
