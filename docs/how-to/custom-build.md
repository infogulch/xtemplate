# How to make a custom build of xtemplate

The published binaries and Docker image include a sqlite database driver and the [core providers](../reference/glossary.md#providers). Copy a small `main` package when you need different drivers, extra FuncMaps, custom providers, or embedded templates.

xtemplate's packaging is intentionally like Caddy's: a thin `main` that blank-imports optional packages and calls into a shared library.

## Default published builds

| Artifact | Entry | Notes |
|---|---|---|
| GitHub release CLI | `./cmd/watchfs` | Live-reloads templates; binary named `xtemplate` in archives |
| Plain CLI | `./cmd` | Same providers, no filesystem watch; used by Docker |
| Git CLI | `./cmd/git` | Poll a Git remote; shallow-clone + reload on new commit |
| Docker image | `infogulch/xtemplate` | Builds `./cmd` (plain) with listen default `:80` |
| Caddy module (lean) | `caddy` | Core handler + Caddyfile surface; add `providers/*/caddyfile` as needed |
| Caddy module (standard) | `caddy/standard` | Lean + Caddyfile parsers for sql, fs, flags, nats + pure-Go sqlite3 driver |

Which variant to pick (including Caddy standard vs lean builds): [Deployment modes](../reference/deployment-modes.md).

## Choose an app

| Package | Behavior |
|---|---|
| `github.com/infogulch/xtemplate/app` | Parse flags/JSON, serve once |
| `github.com/infogulch/xtemplate/app/watchfs` | Same + reload when templates (and `--watch` dirs) change |
| `github.com/infogulch/xtemplate/app/git` | Load/reload templates from a Git remote (`--git-repo`) |

Start from the matching `cmd/*` file.

## Choose providers and drivers

Default `cmd/watchfs/main.go`:

```go
package main

import (
	"github.com/infogulch/xtemplate/app/watchfs"

	_ "github.com/infogulch/xtemplate/providers/dotflags"
	_ "github.com/infogulch/xtemplate/providers/dotfs"
	_ "github.com/infogulch/xtemplate/providers/dotnats"
	_ "github.com/infogulch/xtemplate/providers/dotsql"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func main() {
	watchfs.Main()
}
```

To add another SQL driver, blank-import it instead of sqlite3:

```go
_ "github.com/jackc/pgx/v5/stdlib" // driver name "pgx"
```

Then point the SQL provider at that driver name in config:

```json
{
  "type": "sql",
  "name": "DB",
  "driver": "pgx",
  "connstr": "postgres://..."
}
```

Omit provider imports you do not need to reduce the binary size.

## Pass options from main

```go
func main() {
	watchfs.Main(
		xtemplate.WithProvider(myProvider{}),
		xtemplate.WithFuncMaps(template.FuncMap{
			"myFunc": func(s string) string { return s },
		}),
	)
}
```

To add **new CLI flags or JSON keys** (not just `With*` overrides), embed `app.Config` in your own struct and call `app.LoadConfig`, matching the pattern used by watchfs and git. See [CLI reference - Extending the app config](../reference/cli.md#extending-the-app-config).

## Embed templates (single binary)

Use `//go:embed` and `WithTemplateFS` so no templates directory is required at runtime. Reloading will not be needed, so use the `app` package.

```go
//go:embed all:templates
var templatesFS embed.FS

func main() {
	sub, err := fs.Sub(templatesFS, "templates")
	if err != nil {
		panic(err)
	}
	app.Main(xtemplate.WithTemplateFS(afero.FromIOFS{FS: sub}))
}
```

Full example: [`examples/embedded`](../../examples/embedded/).

## Caddy custom builds

```shell
# Full standard set
xcaddy build \
  --with github.com/infogulch/xtemplate/caddy/standard

# Leaner: core module + selected provider Caddyfile packages + driver as needed
xcaddy build \
  --with github.com/infogulch/xtemplate/caddy \
  --with github.com/infogulch/xtemplate/providers/dotsql/caddyfile \
  --with github.com/ncruces/go-sqlite3/driver
```

See [`caddy/README.md`](../../caddy/README.md).

## Related

- [Create a custom dot provider](create-a-provider.md)
- [CLI reference](../reference/cli.md)
- [Configuration](../reference/configuration.md)
