# CLI reference

Flag inventory for the standalone binaries, and how the `app` package lets you extend that surface.

Related:

- When to use each variant and how to install or run them: [Deployment modes](deployment-modes.md).
- Field meanings, providers, and JSON/Caddyfile/library shapes: [Configuration](configuration.md).
- Custom binaries: [Custom build](../how-to/custom-build.md).

## Binaries

Three thin entries share most flags; they differ in template source and reload:

| Entry | Reload |
|---|---|
| [`cmd/watchfs`](../../cmd/watchfs) | Filesystem watch (default for local work) |
| [`cmd`](../../cmd) | None |
| [`cmd/git`](../../cmd/git) | Git poll (`--git-repo`, `--git-ref`, `--git-interval`) |

```shell
go build -o xtemplate ./cmd/watchfs
```

## Extending the app config

The published CLIs are not three separate flag parsers. They share [`app.LoadConfig`](../../app/app.go), which loads **any** struct that implements `app.Configurable` - typically by **embedding** `app.Config` and adding fields.

`app.Config` itself embeds `xtemplate.Config` and adds listen, log level, and the `-c` / `-f` config sources:

```go
type Config struct {
	xtemplate.Config
	Listen      string   `json:"listen" arg:"-l"`
	LogLevel    int      `json:"log_level" default:"-2"`
	Configs     []string `json:"-" arg:"-c,--config,separate"`
	ConfigFiles []string `json:"-" arg:"-f,--config-file,separate"`
}
```

A variant adds options by embedding that type and tagging new fields for both JSON and [go-arg](https://github.com/alexflint/go-arg). watchfs adds extra watch dirs:

```go
// app/watchfs - simplified
type Config struct {
	app.Config
	Watch []string `json:"watch_dirs" arg:",separate"`
}

var _ app.Configurable = (*Config)(nil)

func Main(options ...xtemplate.Option) {
	config, err := app.LoadConfig(&Config{}, nil)
	// … wire Reload from config.Watch, then:
	app.Serve(&config.Config, options...)
}
```

git does the same with `--git-repo`, `--git-ref`, and `--git-interval` (see [`app/git`](../../app/git/gitapp.go)). Override `SetDefaults` on your outer struct when new fields need defaults; call the embedded `Config.SetDefaults()` so listen/logger still initialize.

Because `LoadConfig` parses and unmarshals into **your** struct value:

- New fields appear as CLI flags and JSON keys automatically (via `arg` / `json` tags).
- Existing xtemplate and app fields keep working without redeclaring them.
- Help (`Epilogue`) and `Version` can be overridden the same way when needed.

For a one-off binary that only needs extra providers or FuncMaps, prefer `watchfs.Main(xtemplate.With…)` or `app.Main(…)` from [Custom build](../how-to/custom-build.md). Embed a new `Configurable` when you need **new flags or JSON keys** of your own (another reload policy, a required remote, etc.).

### Config source precedence

Whatever the outer struct is, `LoadConfig` fills it in this order (later wins): defaults → `-f` files in order → `-c` fragments in order → CLI flags. Flags are parsed twice so they still override JSON after files load. Details live in `app.LoadConfig`.

## Flags

Flags map onto the embedded config fields above (plus variant-only fields). Verified against the current `watchfs` / `cmd` help output:

| Flag | Default | Meaning |
|---|---|---|
| `-t`, `--templates-dir`, `--template-dir` | `templates` | Template root path (relative to working directory / FS) |
| `--template-ext` | `.html` | Extension for path-template sources |
| `-m`, `--minify` | `true` | Minify HTML templates at load time (`--minify=false` to disable) |
| `--precompress` | (none) | Pre-compress static files at load (`gzip`, `zstd`, `br`; repeatable) |
| `--ldelim` | `{{` | Left template delimiter |
| `--rdelim` | `}}` | Right template delimiter |
| `-l`, `--listen` | `0.0.0.0:8080` | Listen address (Docker image defaults to `:80`) |
| `--loglevel` | `-2` | `slog` level (numeric; lower is more verbose) |
| `-c`, `--config` | | Inline JSON config (repeatable); later fragments win |
| `-f`, `--config-file` | | JSON config file path (repeatable); later files win |
| `--watch` | | watchfs only: extra directory to watch (repeatable). The templates dir is always watched |
| `--git-repo` | | git only: remote URL (required) |
| `--git-ref` | | git only: branch / tag / commit-ish |
| `--git-interval` | `15s` | git only: poll interval |
| `-h`, `--help` | | Help |
| `--version` | | Version string |

## Examples

```shell
# Listen on port 80
./xtemplate --listen :80

# Custom templates directory (watchfs always reloads when it changes)
./xtemplate --templates-dir public

# Also reload when ./data changes
./xtemplate --watch data

# Custom extension and keep minify on
./xtemplate --template-ext ".go.html" --minify

# Pre-compress static files at startup
./xtemplate --precompress gzip --precompress br
```
