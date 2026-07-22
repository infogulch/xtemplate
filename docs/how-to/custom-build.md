# Custom build

Ship a binary with the drivers, providers, sources, and defaults you need.

## Stock entries

| Build | Path | Notes |
|---|---|---|
| GitHub release / default CLI | `./cmd/xtemplate` | Blank-imports providers + watchfs + git; default `--source-type` `watchfs` |
| Docker image | `infogulch/xtemplate` | Same entry; ldflags set listen `:80` and `defaultSourceType=os` |

## App package

| Import | Role |
|---|---|
| `github.com/infogulch/xtemplate/app` | CLI load + serve (`app.Main`) |

Optional sources (blank-import to register):

| Import | Type string |
|---|---|
| `github.com/infogulch/xtemplate/sources/watchfs` | `watchfs` |
| `github.com/infogulch/xtemplate/sources/git` | `git` |

## Minimal main

Default `cmd/xtemplate/main.go` shape:

```go
package main

import (
	"github.com/infogulch/xtemplate/app"

	_ "github.com/infogulch/xtemplate/providers/dotsql"
	// … other providers …
	_ "github.com/infogulch/xtemplate/sources/git"
	_ "github.com/infogulch/xtemplate/sources/watchfs"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func main() {
	app.Main()
}
```

Pass overrides:

```go
app.Main(
	xtemplate.WithFuncMaps(myFuncs),
	xtemplate.WithProvider(...),
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

Or pick individual `providers/*/caddyfile` and `sources/*/caddyfile` modules.
