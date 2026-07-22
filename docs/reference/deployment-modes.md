# Deployment modes

How to run xtemplate: CLI, Docker, Caddy, or as a Go library.

## Choose a mode

| Goal | Mode |
|---|---|
| Zero setup container | Docker |
| Local dir, reload on file change | CLI / Docker default (`watchfs`) |
| Local dir, no auto-reload | CLI `--controller-type os` |
| Templates from a Git remote, poll and reload | CLI `--controller-type git` |
| Automatic HTTPS, auth, reverse proxy | Caddy module (`caddy/standard` defaults to `watchfs`) |
| Embed in your Go program | Go library (core default `os` unless you import `watchfs`) |

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
| Docker image | `xtemplate` | same; default controller `watchfs`, listen `:80` |

### Default controller type

[`DefaultControllerType`](https://pkg.go.dev/github.com/infogulch/xtemplate#DefaultControllerType) is `"os"` in core. Full CLI and `caddy/standard` set `"watchfs"`.

| Build | Default |
|---|---|
| Core library | `os` |
| `cmd/xtemplate` / Docker / release CLI | `watchfs` |
| `caddy` module alone | `os` |
| `caddy/standard` | `watchfs` |

Override anytime with `--controller-type`, JSON `"controller": {"type":…}`, Caddyfile `controller <type> { }`, or `WithController` / `WithTemplateFS`.

### Controller types (one binary)

| Type | Sticky base | First content | Reload |
|---|---|---|---|
| `os` | local `path` | sync at Server start | none |
| `watchfs` | local `path` | sync at Server start | FS watch → empty `Reload()` on path + `--watch` dirs |
| `git` | none (nil instance → 503) | after first successful `Reload` with clone FS | poll remote; clone + `WithOnClose` cleanup; last-SHA advances only if Reload succeeds |

```shell
# Local with live reload (CLI / Docker default)
xtemplate --listen :8080

# Explicit os (no auto-reload)
xtemplate --controller-type os --templates-dir templates

# Git
xtemplate --controller-type git --git-repo https://example.com/site.git --git-ref main
```

JSON:

```json
{
  "controller": { "type": "watchfs", "path": "templates", "watch_dirs": ["data"] },
  "listen": ":8080"
}
```

```json
{
  "controller": {
    "type": "git",
    "repo": "https://example.com/site.git",
    "ref": "main",
    "interval": "15s",
    "path": "templates"
  }
}
```

## Docker

Image builds `./cmd/xtemplate` with ldflag `defaultListenAddress=0.0.0.0:80` (same blank-imports as the CLI; default controller is `watchfs`).

```shell
docker run --rm -p 8080:80 \
  -v "$PWD/templates:/app/templates:ro" \
  infogulch/xtemplate:latest
```

## Caddy module

With [`caddy/standard`](../../caddy/standard) (typical download / xcaddy line), `watchfs` is linked so the process default is **`watchfs`** when no `controller` block is set. Build with only `xtemplate/caddy` (no controller packages) and the default stays **`os`**.

Use an explicit block when you care:

```Caddyfile
:8080
route {
	xtemplate {
		# No auto-reload:
		controller os {
			path templates
		}
		# Live reload (also the caddy/standard default when omitted):
		# controller watchfs {
		# 	path templates
		# }
	}
}
```

Legacy `templates_dir` / `watch_template_path` hard-reject with migrate errors (pre-1.0).

Build with standard providers + controllers:

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

`Server()` uses `DefaultControllerType` when no controller or TemplateFS is set. Reload-capable controllers (`watchfs`, `git`) require `Server`, not standalone `Instance`.

| API | Role |
|---|---|
| [`app.Main`](https://pkg.go.dev/github.com/infogulch/xtemplate/app) | Unified CLI |
| [`controllers/watchfs`](https://pkg.go.dev/github.com/infogulch/xtemplate/controllers/watchfs) | Watch controller |
| [`controllers/git`](https://pkg.go.dev/github.com/infogulch/xtemplate/controllers/git) | Git controller |
