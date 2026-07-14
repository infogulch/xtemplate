# Configuration

xtemplate configuration appears in three shapes that describe the same ideas: CLI flags, JSON (file or inline), and Caddyfile (plus Caddy JSON). Library users set [`xtemplate.Config`](https://pkg.go.dev/github.com/infogulch/xtemplate#Config) in Go or pass [`Option`](https://pkg.go.dev/github.com/infogulch/xtemplate#Option) funcs.

## Precedence (CLI app)

When using `app.Main` / `watchfs.Main` (or any `app.LoadConfig` caller), later sources win:

1. Built-in defaults (lowest)
2. `--config-file` / `-f` files (in order)
3. `--config` / `-c` inline JSON fragments (in order)
4. CLI flags (highest)

How variants add their own fields to that load path: [CLI reference - Extending the app config](cli.md#extending-the-app-config).

## Core fields

| JSON key | CLI | Default | Description |
|---|---|---|---|
| `templates_dir` | `-t` / `--templates-dir` | `templates` | Template root path |
| `template_extension` | `--template-ext` | `.html` | Extension for path-template sources |
| `minify` | `-m` / `--minify` | `true` | Minify templates at load (`--minify=false` to disable) |
| `precompress` | `--precompress` | `[]` | Static file encodings to generate at load: `gzip`, `zstd`, `br` |
| `left` | `--ldelim` | `{{` | Left delimiter |
| `right` | `--rdelim` | `}}` | Right delimiter |
| `providers` | (JSON / Caddyfile only) | `[]` | Dot provider configs |
| `crossorigin` | (JSON / Caddyfile object) | enabled | CSRF / cross-origin protection (Go 1.25+); see JSON example |

App-only (not on `xtemplate.Config` itself):

| JSON key | CLI | Default | Description |
|---|---|---|---|
| `listen` | `-l` / `--listen` | `0.0.0.0:8080` | HTTP listen address |
| `log_level` | `--loglevel` | `-2` | slog level |
| `watch_dirs` | `--watch` | `[]` | Extra watch paths (watchfs) |

Go-only options (not in JSON): `TemplatesFS`, `FuncMaps`, `Handlers`, `Ctx`, `Logger`, `Reload`, and providers attached via `WithProvider` without going through the type registry.

## CLI

Flag list, extending the app config, and shell examples: [CLI reference](cli.md).

```shell
./xtemplate -t templates -l :8080 -f config.json
```

## JSON

Top-level object unmarshals into the app config (which embeds `xtemplate.Config`). Prefer a config file when provider blocks or shared deploy settings would make CLI flags unwieldy. Runnable examples live under [`examples/*/config.json`](../../examples/). Load order of `-f` / `-c` / flags: [Precedence](#precedence-cli-app).

### Shape

Providers are a list of objects with a `type` discriminator and a `name` (dot field name).

```json
{
  "templates_dir": "templates",
  "template_extension": ".html",
  "minify": true,
  "precompress": ["gzip"],
  "providers": [
    {
      "type": "fs",
      "name": "FS",
      "path": "data"
    },
    {
      "type": "sql",
      "name": "DB",
      "driver": "sqlite3",
      "connstr": "file:./app.sqlite",
      "max_open_conns": 1
    },
    {
      "type": "flags",
      "name": "Flags",
      "values": {
        "env": "production",
        "version": "1.2.3"
      }
    },
    {
      "type": "nats",
      "name": "Nats",
      "nats_config": {
        "in_process_server_options": {
          "dont_listen": true
        }
      }
    },
    {
      "type": "smtp",
      "name": "Email",
      "host": "smtp.example.com",
      "from": "noreply@example.com",
      "username": "smtp-user",
      "password": "smtp-pass"
    }
  ],
  "crossorigin": {
    "disabled": false,
    "trusted_origins": ["https://example.com"],
    "insecure_bypass_patterns": []
  }
}
```

### Provider types

| `type` | Package | Typical fields |
|---|---|---|
| `sql` | `providers/dotsql` | `name`, `driver`, `connstr`, `max_open_conns` |
| `fs` | `providers/dotfs` | `name`, `path`, `writable` (bool, default false) |
| `flags` | `providers/dotflags` | `name`, `values` (string map) |
| `nats` | `providers/dotnats` | `name`, `nats_config`, … |
| `smtp` | `providers/dotsmtp` | `name`, `host`, `from`, `port`, `username`, `password`, `auth`, `tls`, `helo`, `max_recipients`, `max_message_bytes`, `send_timeout` |

Unknown `type` values error at load with a hint to import the registering package. The standard CLI blank-imports all five core providers.

For `smtp`, `send_timeout` is a Go `time.Duration`: in JSON it is a **nanosecond integer** (for example `30000000000` for 30s). Caddyfile `send_timeout 30s` is converted for you.

Multiple providers of the same type are allowed if their `name` fields differ. For example, two SQL databases as separate dot fields:

```json
"providers": [
  { "type": "sql", "name": "DB", "driver": "sqlite3", "connstr": "file:./app.sqlite" },
  { "type": "sql", "name": "Analytics", "driver": "pgx", "connstr": "postgres://..." }
]
```

Templates then use `{{.DB.QueryRows ...}}` and `{{.Analytics.QueryRows ...}}`. The `driver` name must be registered in the binary via a blank import; stock builds include only `sqlite3`. See [Custom build](../how-to/custom-build.md).

## Caddyfile

```Caddyfile
route {
	xtemplate {
		templates_dir templates
		template_extension .html
		minify true
		delimiters {{ }}
		precompress gzip br

		crossorigin {
			disabled false
			trusted_origins https://example.com
		}

		provider sql DB {
			driver  sqlite3
			connstr file:./app.sqlite
		}
		provider fs FS {
			path data
			# writable true   # enables ReceiveFiles; root must be writable
		}
		provider flags Flags {
			env production
		}
	}
}
```

Details and advanced JSON: [`caddy/README.md`](../../caddy/README.md).

## Library (Go)

```go
cfg := xtemplate.New()
cfg.TemplatesDir = "templates"
// or: xtemplate.WithTemplateFS(...), WithProvider(...), WithFuncMaps(...), WithHandler(...)

srv, err := cfg.Server()
// inst, _, _, err := cfg.Instance()
```

`app.Main(overrides...)` / `watchfs.Main(overrides...)` apply the same options after loading CLI/JSON, which is how [custom builds](../how-to/custom-build.md) inject providers and embedded FS.
