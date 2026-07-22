# xtemplate

**Hypermedia web apps from a directory of Go templates.**

xtemplate is a Go server that treats templates as handlers: file-based routing, a request-scoped dot context, and first-class static files; no separate application layer.

```html
<!-- contacts.html  →  GET /contacts (SQL provider configured as .DB) -->
<ul>
  {{range .DB.QueryRows `SELECT id, name FROM contacts`}}
  <li><a href="/contacts/{{.id}}">{{.name}}</a></li>
  {{end}}
</ul>
```

```html
<!-- contacts/{id}.html  →  GET /contacts/{id} -->
{{$id := .Req.PathValue `id`}}
{{$c := .DB.QueryRow `SELECT id, name, phone, email FROM contacts WHERE id = ?` $id}}
<form method="POST">
  <input name="name"  value="{{$c.name}}">
  <input name="phone" value="{{$c.phone}}">
  <input name="email" value="{{$c.email}}">
  <button>Update</button>
</form>

{{define "POST /contacts/{id}"}}
{{$_ := .DB.Exec `UPDATE contacts SET name=?, phone=?, email=? WHERE id = ?`
    (.Req.FormValue `name`) (.Req.FormValue `phone`)
    (.Req.FormValue `email`) (.Req.PathValue `id`)}}
{{template `/contacts/{id}.html` .}}
{{end}}
```

No route table, no handler functions. One file for the index. One file for the form and the update. Path parameters, form values, and SQL all hang off the [dot context](docs/reference/dot-context.md).

## Philosophy

Deal with the first-class citizens of the web: paths, requests, HTML responses, and backing data. Templates are expressive enough to be the app. More details in [Design](docs/explanation/design.md).

## Highlights

- **File-based routing** - `admin/settings.html` serves `GET /admin/settings`; `index.html` serves the directory. [Template semantics](docs/reference/template-semantics.md)
- **Any method and pattern** - `{{define "DELETE /contact/{id}"}}` is a route. [Instance loading](docs/reference/instance-loading.md)
- **Loads once, reloads live** - parse at startup; swap an immutable instance on change. [Design](docs/explanation/design.md)
- **Dot context** - `.Req`, `.Resp`, `.DB`, `.FS`, … per request. [Dot context](docs/reference/dot-context.md)
- **Safe by default** - `html/template` escaping; optional sanitize / trust helpers. [Functions](docs/reference/functions.md)
- **Optimal static files** - hashes, SRI, precompressed encodings, long cache. [Instance loading](docs/reference/instance-loading.md#static-files)
- **SSE** - `{{define "SSE /path"}}` for live updates. [Dot context → Flush](docs/reference/dot-context.md#streaming-control-in-flush)
- **Embeddable** - CLI, Docker, Caddy plugin, or `http.Handler` library. [Deployment modes](docs/reference/deployment-modes.md)

Some more patterns:

```html
{{- with $hash := .X.StaticFileHash `/assets/reset.css`}}
<link rel="stylesheet" href="/assets/reset.css?hash={{$hash}}" integrity="{{$hash}}">
{{- end}}
```

```html
{{- define "SSE /reload"}}{{.Flush.WaitForServerStop}}data: reload{{printf "\n\n"}}{{end}}
<script>new EventSource("/reload").onmessage = () => location.reload()</script>
```

More complete apps: [`examples/`](examples/).

## Quick start
| If you want… | Start here |
|---|---|
| Step-by-step first app | [Getting started tutorial](docs/tutorial/getting-started.md) |
| Zero setup container | [Docker](docs/reference/deployment-modes.md#docker) |
| Local templates + live reload | [CLI](docs/reference/deployment-modes.md) · [CLI flags](docs/reference/cli.md) |
| Templates from a Git remote | [CLI `--source-type git`](docs/reference/deployment-modes.md) |
| Automatic HTTPS, auth, proxy | [Caddy module](docs/reference/deployment-modes.md#caddy-module) |
| Embed in your Go program | [Go library](docs/reference/deployment-modes.md#go-library) |

```shell
# CLI (live reload)
go install github.com/infogulch/xtemplate/cmd/xtemplate@latest
mkdir -p templates && echo '<h1>{{.Req.URL.Path}}</h1>' > templates/index.html
xtemplate --listen :8080
# open http://localhost:8080
```

```shell
# Docker
docker run --rm -p 8080:80 \
  -v "$PWD/templates:/app/templates:ro" \
  infogulch/xtemplate:latest
```

```Caddyfile
# Caddy
:8080
route {
	xtemplate
}
```

Full integration map and configs: **[docs/reference/deployment-modes.md](docs/reference/deployment-modes.md)**.

All documentation (tutorial, how-tos, reference, design): **[docs/](docs/)**.

## Users

- [PixyBlue/lazy-lob-web](https://github.com/PixyBlue/lazy-lob-web) - fullstack web lob framework
- [infogulch/xrss](https://github.com/infogulch/xrss) - RSS reader with htmx
- [infogulch/todos](https://github.com/infogulch/todos) - TodoMVC demo

## Contributing

Development setup, repo map, and tests: [CONTRIBUTING.md](CONTRIBUTING.md) (same as [docs/contributing.md](docs/contributing.md)).

## History and license

Evolved from [go-htmx](https://github.com/infogulch/go-htmx) and a Caddy-centric prototype into a standalone library with an optional Caddy module. Narrative: [Project history](docs/explanation/history.md). Releases: [CHANGELOG.md](CHANGELOG.md).

Licensed under the Apache 2.0 license. See [LICENSE](./LICENSE).
