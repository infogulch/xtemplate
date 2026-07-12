# Getting started

Welcome! In this short tutorial you will create a minimal dynamic website with xtemplate and have it running in a few minutes.

xtemplate turns a directory of Go `html/template` files into a complete web server. File paths become routes, templates become handlers, and you stay close to HTTP and hypermedia.

By the end you will have:

- A running xtemplate server
- File-based routing from a path template
- A define-based route via `{{define}}`
- Live reload while you edit the template root

## Prerequisites

You need one of:

- A pre-built `xtemplate` binary from the [releases page](https://github.com/infogulch/xtemplate/releases)
- Go 1.25+ (to build from source)

## 1. Create your project

```bash
mkdir my-first-site && cd my-first-site
mkdir templates
```

Create `templates/index.html`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Hello from xtemplate</title>
    <style>
        body { font-family: system-ui, sans-serif; margin: 2rem; }
        .info { background: #f0f0f0; padding: 1rem; border-radius: 6px; }
    </style>
</head>
<body>
    <h1>Hello from xtemplate!</h1>

    <div class="info">
        <p><strong>Path:</strong> {{.Req.URL.Path}}</p>
        <p><strong>Method:</strong> {{.Req.Method}}</p>
        <p><strong>Remote address:</strong> {{.Req.RemoteAddr}}</p>
    </div>

    <p>Try editing this file and refreshing the browser - the server will pick
    up your changes automatically.</p>
</body>
</html>
```

## 2. Run xtemplate

Download a release, or build the default live-reload binary:

```bash
go install github.com/infogulch/xtemplate/cmd/watchfs@latest
# or from a checkout of the repo:
go build -o xtemplate ./cmd/watchfs
```

Run it from your project directory (so `./templates` resolves):

```bash
# if installed via go install, the binary is named watchfs
watchfs --listen :8080

# if you built -o xtemplate:
./xtemplate --listen :8080
```

Open http://localhost:8080 - you should see your page. The file `templates/index.html` handles `GET /`.

## 3. Experience live reload

1. Keep the server running.
2. Edit `templates/index.html` (change the heading or add text).
3. Save the file.
4. Refresh the browser.

The change appears without restarting. The watchfs build watches the template root and reloads the instance automatically. A failed load keeps the previous instance serving and logs the error.

## 4. Add a define-based route

Create `templates/hello.html`:

```html
<!DOCTYPE html>
<html>
{{- $name := .Req.FormValue "name" | default "World"}}
<head><title>Hello {{$name}}!</title></head>
<body>
    <h1>Hello {{$name}}!</h1>
    <p>This page uses the query or form field <code>name</code>.</p>
    <form method="POST" action="/hello">
        <input type="text" name="name" placeholder="Enter your name" value="{{$name}}">
        <button type="submit">Submit</button>
    </form>
    <p><a href="/">Back home</a></p>
</body>
</html>

{{- define "POST /hello"}}
{{- template "/hello.html" .}}
{{- end}}
```

Save and visit http://localhost:8080/hello. Submit the form: the path template handles `GET /hello`; the define template `{{define "POST /hello"}}` handles `POST` - no separate Go handler.

User input is HTML-escaped by default.

## 5. Explore the dot context

Add to `index.html`:

```html
<p>Headers: {{len .Req.Header}} headers present</p>
<p>User agent: {{.Req.UserAgent}}</p>
```

Save and refresh. Request data is on `.Req` (builtin provider); response control is on `.Resp` for buffered handlers. More: [Dot context](../reference/dot-context.md).

## 6. What's next?

You have a working site with live reload and define-based routing.

Core concepts

- [Template semantics](../reference/template-semantics.md)
- [Dot context](../reference/dot-context.md)
- [Template functions](../reference/functions.md)
- [Glossary](../reference/glossary.md)

Real functionality

- [Configuration](../reference/configuration.md) (CLI flags, JSON, Caddyfile)
- [SQL and other providers](../reference/dot-context.md#core-providers-configured-dot-fields)
- Example apps under [`examples/`](../../examples/) (contacts, blog, SSE chat, …)

Deploy

- [Deployment modes](../reference/deployment-modes.md) (Docker, Caddy, CLI, library)
- [CLI reference](../reference/cli.md)

Design

- [Design explanation](../explanation/design.md)

---

Feedback welcome: open an issue or PR on the [xtemplate repository](https://github.com/infogulch/xtemplate).
