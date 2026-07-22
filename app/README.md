# xtemplate `app`

CLI application layer: parse flags and JSON, construct an `xtemplate.Server`, and serve. The core library stays free of CLI concerns.

One package (`app`) and one main (`cmd/xtemplate`) cover all built-in template sources via `--source-type` / JSON `"source"`:

| Source type | Package | Role |
|---|---|---|
| `os` (library/Caddy/Docker default) | core | Serve from a local directory; no reload |
| `watchfs` (CLI release default) | `sources/watchfs` | Reload when the template root (and `--watch` dirs) change |
| `git` | `sources/git` | Load/reload templates from a Git remote |

`cmd/xtemplate` blank-imports providers, drivers, and optional sources, then calls `app.Main`.

## Customize a binary

Copy `cmd/xtemplate` into your own module when you need different drivers, FuncMaps, providers, or defaults. Pass `xtemplate.Option`s into `Main`:

```go
package main

import (
	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/app"

	_ "github.com/infogulch/xtemplate/providers/dotsql"
	_ "github.com/infogulch/xtemplate/sources/watchfs"
	_ "github.com/jackc/pgx/v5/stdlib" // driver name "pgx"
)

func main() {
	app.Main(
		// xtemplate.WithProvider(...),
		// xtemplate.WithFuncMaps(...),
	)
}
```

## Docs

- [CLI reference](../docs/reference/cli.md) - flags, source types
- [Configuration](../docs/reference/configuration.md) - field catalog, JSON, Caddyfile
- [Deployment modes](../docs/reference/deployment-modes.md) - install, Docker, Caddy
- [Custom build](../docs/how-to/custom-build.md) - drivers, embed, providers
