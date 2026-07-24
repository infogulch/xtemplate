# CLI reference

Flag inventory for the standalone binary, and how the `app` package loads config.

Related:

- When to use each source type and how to install or run: [Deployment modes](deployment-modes.md).
- Field meanings, providers, and JSON/Caddyfile/library shapes: [Configuration](configuration.md).
- Custom binaries: [Custom build](../how-to/custom-build.md).

## Binary

One entrypoint: [`cmd/xtemplate`](../../cmd/xtemplate). It blank-imports core providers and optional sources (`watchfs`, `git`). Choose the template source with `--source-type` or JSON `"source": {"type":…}`.

| Build | Default `--source-type` |
|---|---|
| `cmd/xtemplate` / release | `watchfs` |
| Docker | `os` (ldflag `defaultSourceType`) |
| Library / Caddy (no source block) | `os` |

```shell
go install github.com/infogulch/xtemplate/cmd/xtemplate@latest
# or
go build -o xtemplate ./cmd/xtemplate
```

## Config loading

[`app.LoadConfig`](../../app/app.go) scans argv (pass 0) for `--source-type`, `-f`/`--config-file`, `-c`/`--config`, then loads JSON (with a ban-list for legacy keys), picks the effective source type, and parses CLI flags into the app config plus the effective source dest (pass B). After load, `Source` is materialized and `SourceRaw` is cleared.

Precedence (later wins): defaults → `-f` files → `-c` fragments → CLI flags.

JSON `"source"."type"` must match `--source-type` when both are set.

## Flags

### App-level

| Flag | Default | Meaning |
|---|---|---|
| `-l`, `--listen` | `0.0.0.0:8080` | Listen address (Docker often `:80` via ldflag) |
| `--loglevel` | `-2` | `slog` level (numeric; lower is more verbose) |
| `-c`, `--config` | | Inline JSON (repeatable); later wins |
| `-f`, `--config-file` | | JSON config file (repeatable); later wins |
| `--source-type` | build default (table above) | Active source type; must match JSON `source.type` if both set |
| `-h`, `--help` | | Help (lists registered source types) |
| `--version` | | Version string |

### Core / shared template options

| Flag | Applies when | Default | Meaning |
|---|---|---|---|
| `-t`, `--templates-dir`, `--template-dir` | `os`, `watchfs`, `git` (subdir) | `templates` | Path on that source |
| `--template-ext` | always | `.html` | Extension for path-template sources |
| `-m`, `--minify` | always | `true` | Minify HTML templates at load (`--minify=false` to disable) |
| `--precompress` | always | none | Pre-compress static files (`gzip`, `zstd`, `br`; repeatable) |
| `--ldelim` / `--rdelim` | always | `{{` / `}}` | Template delimiters |

### Source-specific (only for the effective type)

| Flag | Type | Default | Meaning |
|---|---|---|---|
| `--watch` | `watchfs` | none | Extra watch dirs (repeatable); templates `Path` always watched |
| `--debounce` | `watchfs` | `200ms` | FS event debounce |
| `--git-repo` | `git` | required | Repository URL or path |
| `--git-ref` | `git` | (remote default) | Branch / tag / ref |
| `--git-interval` | `git` | `15s` | Poll interval |

## Examples

```shell
# Listen on port 80 (default source watchfs in release builds)
./xtemplate --listen :80

# Explicit os source (no reload)
./xtemplate --source-type os --templates-dir public

# watchfs: also reload when ./data changes
./xtemplate --source-type watchfs --watch data

# git source
./xtemplate --source-type git --git-repo https://example.com/site.git --git-ref main

# Config file
./xtemplate -f config.json
```
