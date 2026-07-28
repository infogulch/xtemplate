# Configuration

xtemplate configuration appears in three shapes that describe the same ideas: CLI flags, JSON (file or inline), and Caddyfile (plus Caddy JSON). Library users set [`xtemplate.Config`](https://pkg.go.dev/github.com/infogulch/xtemplate#Config) in Go or pass [`Option`](https://pkg.go.dev/github.com/infogulch/xtemplate#Option) funcs.

## Precedence (CLI app)

When using `app.Main` / `app.LoadConfig`, later sources win:

1. Built-in defaults (lowest)
2. `--config-file` / `-f` files (in order)
3. `--config` / `-c` inline JSON fragments (in order)
4. CLI flags (highest)

Details: [CLI reference](cli.md).

## Core fields

| JSON key | CLI | Default | Description |
|---|---|---|---|
| `controller` | `--controller-type` + type flags | `DefaultControllerType` (core `os`; CLI often `watchfs`) | Template controller object (`type` + fields) |
| `template_extension` | `--template-ext` | `.html` | Extension for path-template files |
| `minify` | `-m` / `--minify` | `true` | Minify templates at load (`--minify=false` to disable) |
| `precompress` | `--precompress` | `[]` | Static file encodings: `gzip`, `zstd`, `br` |
| `left` | `--ldelim` | `{{` | Left delimiter |
| `right` | `--rdelim` | `}}` | Right delimiter |
| `providers` | (JSON / Caddyfile only) | `[]` | Dot provider configs |
| `crossorigin` | (JSON / Caddyfile object) | enabled | CSRF / cross-origin protection |

App-only (not on `xtemplate.Config` itself):

| JSON key | CLI | Default | Description |
|---|---|---|---|
| `listen` | `-l` / `--listen` | `0.0.0.0:8080` | HTTP listen address |
| `log_level` | `--loglevel` | `-2` | slog level |

Go-only options (not in JSON): `Controller`, `FuncMaps`, `Handlers`, `Ctx`, `Logger`, and providers via `WithProvider`. Dual-write: `WithTemplateFS`, `WithTemplateDir`.

### Controller types

| `type` | Package | Typical fields |
|---|---|---|
| `os` | core | `path` (default `templates`) |
| `watchfs` | `controllers/watchfs` | `path`, `watch_dirs`, `debounce` |
| `git` | `controllers/git` | `repo`, `ref`, `interval`, `path` (subdir in clone) |

When `controller` / `--controller-type` is omitted, the effective type is [`DefaultControllerType`](https://pkg.go.dev/github.com/infogulch/xtemplate#DefaultControllerType). In-process template roots use `WithTemplateFS` / `WithTemplateDir` (no controller type).

### Legacy keys (hard-reject before 1.0)

These top-level JSON keys fail load with a migrate message (not silent ignore):

| Banned key | Use instead |
|---|---|
| `templates_dir` | `"controller": {"type":"os","path":"…"}` (or watchfs/git) |
| `watch_dirs` | `"controller": {"type":"watchfs","path":"…","watch_dirs":[…]}` |
| `watch_template_path` | `"controller": {"type":"watchfs",…}` or omit when `DefaultControllerType` is already `watchfs` |

Other unknown JSON keys are still ignored (`encoding/json` contract).

## CLI

Flag list and controller-type selection: [CLI reference](cli.md).

```shell
./xtemplate -t templates -l :8080 -f config.json
```

## JSON

```json
{
  "controller": {"type": "os", "path": "templates"},
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
      "type": "bus",
      "name": "Bus",
      "buffer": 16
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

Watchfs / git examples:

```json
{ "controller": { "type": "watchfs", "path": "templates", "watch_dirs": ["data"], "debounce": "200ms" } }
{ "controller": { "type": "git", "repo": "https://example.com/site.git", "ref": "main", "interval": "30s", "path": "templates" } }
```

### Provider types

| `type` | Package | Typical fields |
|---|---|---|
| `sql` | `providers/dotsql` | `name`, `driver`, `connstr`, `max_open_conns` |
| `fs` | `providers/dotfs` | `name`, `path`, `writable` (bool, default false) |
| `flags` | `providers/dotflags` | `name`, `values` (string map) |
| `bus` | `providers/dotbus` | `name`, `buffer` (int, default 16) |
| `nats` | `providers/dotnats` | `name`, `nats_config`, … |
| `smtp` | `providers/dotsmtp` | `name`, `host`, `from`, `port`, …, `send_timeout` |

Unknown `type` values error at load with a hint to import the registering package. The standard CLI blank-imports all core providers.

Duration fields (`send_timeout`, controller `interval` / `debounce`, etc.) accept a
JSON **string** only (e.g. `"30s"`, `"1m"`). Numeric nanosecond integers are
rejected.

Multiple providers of the same type are allowed if their `name` fields differ.

## Caddyfile

Default without a `controller` block is **os** (no watch). Use `controller watchfs` for reload-on-change.

Legacy directives `templates_dir` / `templates_path` / `watch_template_path` hard-reject with migrate errors.

```Caddyfile
route {
	xtemplate {
		controller os {
			path templates
		}
		# or: controller watchfs { path templates }
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
		}
		provider flags Flags {
			env production
		}
	}
}
```

Details: [`caddy/README.md`](../../caddy/README.md).

## Library (Go)

```go
cfg := xtemplate.New()
// Server() uses DefaultControllerType when no Controller / TemplateFS, or:
//   xtemplate.WithTemplateDir("templates")
//   xtemplate.WithTemplateFS(fs)
//   xtemplate.WithController(&xtemplate.OsFsController{Path: "templates"})

srv, err := cfg.Server()
// inst, err := cfg.Instance() // requires WithTemplateFS/Dir
```

`app.Main(overrides...)` applies the same options after loading CLI/JSON.
