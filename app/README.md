# xtemplate `app`

CLI application layer: parse flags and JSON, construct an `xtemplate.Server`, and serve. The core library stays free of CLI concerns.

| Package | Role |
|---|---|
| `app` | plain CLI — load config, serve once (`app.Main`) |
| `app/watchfs` | same + reload when the template root (and `--watch` dirs) change |
| `app/git` | load/reload templates from a Git remote |

Thin `main` packages under [`cmd/`](../cmd/), [`cmd/watchfs/`](../cmd/watchfs/), and [`cmd/git/`](../cmd/git/) blank-import providers/drivers and call the matching `Main`.

## Customize a binary

Copy a `cmd/*` entry into your own module when you need different drivers, FuncMaps, providers, or defaults. Pass `xtemplate.Option`s into `Main`:

```go
package main

import (
	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/app/watchfs"

	_ "github.com/infogulch/xtemplate/providers/dotsql"
	_ "github.com/jackc/pgx/v5/stdlib" // driver name "pgx"
)

func main() {
	watchfs.Main(
		// xtemplate.WithProvider(...),
		// xtemplate.WithFuncMaps(...),
	)
}
```

To add **new flags or JSON keys**, embed `app.Config` and use `app.LoadConfig` (same pattern as watchfs/git). See the docs links below.

## Docs

- [CLI reference](../docs/reference/cli.md) — flags, extending the app config
- [Configuration](../docs/reference/configuration.md) — field catalog, JSON, Caddyfile
- [Deployment modes](../docs/reference/deployment-modes.md) — install, variants, Docker
- [Custom build](../docs/how-to/custom-build.md) — drivers, embed, providers
