# CLI reference

Flag inventory for the standalone binary, and how the `app` package loads config.

Related:

- When to use each controller type and how to install or run: [Deployment modes](deployment-modes.md).
- Field meanings, providers, and JSON/Caddyfile/library shapes: [Configuration](configuration.md).
- Custom binaries: [Custom build](../how-to/custom-build.md).

## Binary

One entrypoint: [`cmd/xtemplate`](../../cmd/xtemplate). It blank-imports core providers and optional controllers (`watchfs`, `git`), then sets [`DefaultControllerType`](https://pkg.go.dev/github.com/infogulch/xtemplate#DefaultControllerType) to **`watchfs`**, so the release CLI defaults to live reload. Override with `--controller-type` or JSON `"controller": {"type":…}`.

```shell
go install github.com/infogulch/xtemplate/cmd/xtemplate@latest
# or
go build -o xtemplate ./cmd/xtemplate
```

## Config loading

[`app.LoadConfig`](../../app/app.go) runs a fixed pipeline: argv bootstrap (pass 0: `--controller-type`, `-f`/`--config-file`, `-c`/`--config`) → merge JSON (legacy-key ban-list) → materialize controller (CLI `--controller-type` > JSON `controller.type` > [`DefaultControllerType`](https://pkg.go.dev/github.com/infogulch/xtemplate#DefaultControllerType)) via [`Config.MaterializeController`](https://pkg.go.dev/github.com/infogulch/xtemplate#Config.MaterializeController) → parse CLI flags into app + controller dest (go-arg handles `--help` / `--version`) → finalize logger. After load, `Controller` is set and `ControllerRaw` is cleared. Pass 0 is hand-scanned because go-arg needs the concrete controller type before full parse (type-specific flags).

Effective type when omitted from CLI and JSON: [`xtemplate.DefaultControllerType`](https://pkg.go.dev/github.com/infogulch/xtemplate#DefaultControllerType) (core `"os"`; release CLI sets `"watchfs"`).

Precedence (later wins): defaults → `-f` files → `-c` fragments → CLI flags.

When both JSON `"controller"."type"` and `--controller-type` are set, **CLI wins**. If the types differ, JSON `ControllerRaw` is discarded and flags bind to a zero controller of the CLI type (path and other fields from JSON are not carried over).

## Flags

### App-level

| Flag | Default | Meaning |
|---|---|---|
| `-l`, `--listen` | `0.0.0.0:8080` | Listen address (Docker often `:80` via ldflag) |
| `--loglevel` | `-2` | `slog` level (numeric; lower is more verbose) |
| `-c`, `--config` | | Inline JSON (repeatable); later wins |
| `-f`, `--config-file` | | JSON config file (repeatable); later wins |
| `--controller-type` | `DefaultControllerType` | Active controller type; CLI wins over JSON `controller.type` when both set |
| `-h`, `--help` | | Help (lists registered controller types and the effective default) |
| `--version` | | Version string |

### Core / shared template options

| Flag | Applies when | Default | Meaning |
|---|---|---|---|
| `-t`, `--templates-dir`, `--template-dir` | `os`, `watchfs`, `git` (subdir) | `templates` | Path on that controller |
| `--template-ext` | always | `.html` | Extension for path-template files |
| `-m`, `--minify` | always | `true` | Minify HTML templates at load (`--minify=false` to disable) |
| `--precompress` | always | none | Pre-compress static files (`gzip`, `zstd`, `br`; repeatable) |
| `--ldelim` / `--rdelim` | always | `{{` / `}}` | Template delimiters |

### Controller-specific (only for the effective type)

| Flag | Type | Default | Meaning |
|---|---|---|---|
| `--watch` | `watchfs` | none | Extra watch dirs (repeatable); templates `Path` always watched |
| `--debounce` | `watchfs` | `200ms` | FS event debounce |
| `--git-repo` | `git` | required | Repository URL or path |
| `--git-ref` | `git` | (remote default) | Branch / tag / ref |
| `--git-interval` | `git` | `15s` | Poll interval |

## Examples

```shell
# Listen on port 80 (release CLI default is watchfs — live reload)
./xtemplate --listen :80

# Templates from a custom directory
./xtemplate --templates-dir public

# No auto-reload
./xtemplate --controller-type os

# watchfs explicitly; also watch ./data
./xtemplate --controller-type watchfs --watch data

# git controller
./xtemplate --controller-type git --git-repo https://example.com/site.git --git-ref main

# Config file
./xtemplate -f config.json
```
