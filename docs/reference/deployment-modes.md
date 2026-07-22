# Deployment modes

How to run xtemplate: CLI, Docker, Caddy, or as a Go library.

## Choose a mode

| Goal | Mode |
|---|---|
| Zero setup container | Docker |
| Local dir, no auto-reload | CLI `--source-type os` (or Docker default) |
| Local dir, reload on file change | CLI default `watchfs` |
| Templates from a Git remote, poll and reload | CLI `--source-type git` |
| Automatic HTTPS, auth, reverse proxy | Caddy module (default source `os`) |
| Embed in your Go program | Go library |

## Install / build CLI

```shell
go install github.com/infogulch/xtemplate/cmd/xtemplate@latest

# or from a checkout
go build -o xtemplate ./cmd/xtemplate
```

| Artifact | Binary name | Entry |
|---|---|---|
| GitHub release archives | `xtemplate` | `./cmd/xtemplate` |
| `go install …/cmd/xtemplate` | `xtemplate` | last path segment |
| Docker image | `xtemplate` | same; default source `os`, listen `:80` |

### Source types (one binary)

| Type | Default where | Template root | Reload |
|---|---|---|---|
| `os` | library, Caddy, Docker | local `path` | none |
| `watchfs` | release CLI | local `path` | FS watch on path + `--watch` dirs |
| `git` | opt-in | clone subdir `path` | poll remote; clone + `WithOnClose` cleanup |

```shell
# Local with live reload (CLI release default)
xtemplate --listen :8080

# Explicit os
xtemplate --source-type os --templates-dir templates

# Git
xtemplate --source-type git --git-repo https://example.com/site.git --git-ref main
```

JSON:

```json
{
  "source": { "type": "watchfs", "path": "templates", "watch_dirs": ["data"] },
  "listen": ":8080"
}
```

```json
{
  "source": {
    "type": "git",
    "repo": "https://example.com/site.git",
    "ref": "main",
    "interval": "15s",
    "path": "templates"
  }
}
```

## Docker

Image builds `./cmd/xtemplate` with ldflags `defaultListenAddress=0.0.0.0:80` and `defaultSourceType=os`.

```shell
docker run --rm -p 8080:80 \
  -v "$PWD/templates:/app/templates:ro" \
  infogulch/xtemplate:latest
```

## Caddy module

**Caddy no longer watches by default.** Default source is `os`. Use `source watchfs { … }` for reload-on-change. Legacy `templates_dir` / `watch_template_path` hard-reject with migrate errors (pre-1.0).

```Caddyfile
:8080
route {
	xtemplate {
		source os {
			path templates
		}
		# For live reload:
		# source watchfs {
		# 	path templates
		# }
	}
}
```

Build with standard providers + sources:

```shell
xcaddy build --with github.com/infogulch/xtemplate/caddy/standard
```

Provision calls `Server()`; Cleanup calls `Stop()`.

## Go library

```go
cfg := xtemplate.New()
// optional: cfg, _ = cfg.Options(xtemplate.WithTemplateDir("templates"))
srv, err := cfg.Server()
if err != nil {
	log.Fatal(err)
}
log.Fatal(srv.Serve(":8080"))
```

Reload-capable sources (`watchfs`, `git`) require `Server`, not standalone `Instance`.

| API | Role |
|---|---|
| [`app.Main`](https://pkg.go.dev/github.com/infogulch/xtemplate/app) | Unified CLI |
| [`sources/watchfs`](https://pkg.go.dev/github.com/infogulch/xtemplate/sources/watchfs) | Watch source package |
| [`sources/git`](https://pkg.go.dev/github.com/infogulch/xtemplate/sources/git) | Git source package |
