# Template semantics

xtemplate templates are Go [`html/template`](https://pkg.go.dev/html/template) programs with file- and define-based routing, a uniform per-request [dot context](dot-context.md), and a few extra behaviors. Loading and routing rules are detailed in [Instance loading](instance-loading.md).

## Go template syntax

Actions use the configured delimiters (default `{{` `}}`). Conditionals, ranges, pipelines, variables, and `define` / `block` / `template` work as in the standard library. Prefer the official [text/template](https://pkg.go.dev/text/template) and [html/template](https://pkg.go.dev/html/template) docs for syntax; this page covers xtemplate-specific behavior.

Output is context-aware HTML-escaped by default. See [Design](../explanation/design.md) for the safety posture and the `trust*` / `sanitizeHtml` funcs when you need to opt out carefully.

## Functions and dot context

Two extension mechanisms:

| Mechanism | Lifetime | Typical use |
|---|---|---|
| Template functions (`{{myFunc x}}`) | Set when the instance is built; no request state | Formatting, strings, pure computation |
| Dot fields (`{{.DB.QueryRows ...}}`) | From dot providers, built per request | DB, FS, request/response, I/O |

> [!note]
> Dot fields are initialized on every request with access to the underlying `http.Request` and `http.ResponseWriter`, the request-scoped logger, and the server context. Prefer template functions for simple computational work; dot fields for network, database, and filesystem access.

- [Template functions](functions.md)
- [Dot context](dot-context.md)

## Loops and conditionals

Standard `range`, `if`, `with`, and `else` apply. SQL helpers often return maps or iterators that work directly with `range`:

```html
<ul>
{{range .DB.QueryRows `SELECT id, name FROM contacts ORDER BY name`}}
  <li><a href="/contact/{{.id}}">{{.name}}</a></li>
{{else}}
  <li>No contacts yet.</li>
{{end}}
</ul>
```

## Global template namespace

All templates under the template root share one namespace after load. A name comes from either:

- the file path relative to the root with a leading `/` (path template), or
- an explicit `{{define "name"}}` (define template).

Later definitions with the same name override earlier ones (logged at debug). That means any file can invoke `/shared/.head.html` or a define named `navbar` regardless of directory.

## Invoking templates

```html
<!-- by file path (leading slash, include extension) -->
{{template "/shared/.head.html" .}}

<!-- by defined name -->
{{template "navbar" .}}
```

Pass `.` (or a narrowed value) so nested templates keep the fields they need. Re-rendering a path template after a mutation is a common pattern:

```html
{{define "POST /contacts/{id}"}}
  {{$_ := .DB.Exec `UPDATE contacts SET name=? WHERE id=?`
      (.Req.FormValue "name") (.Req.PathValue "id")}}
  {{template "/contacts/{id}.html" .}}
{{end}}
```

## Path templates and routes

A path template is associated with a `GET` route derived from its file path (extension stripped; `index.html` → directory). Hidden basenames are still parsed into the global namespace but are not given a file-based GET route (so partials like `/shared/.head.html` remain invocable). See [Instance loading](instance-loading.md).

## Define-based routes

`{{define "METHOD /path/{param}"}}` registers a route. Supported methods: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, and the pseudo-method `SSE` (flushing handler). Path parameters use ServeMux syntax and are read with `.Req.PathValue`.

```html
{{define "GET /contact/{id}"}}
{{$contact := .DB.QueryRow `SELECT name, phone FROM contacts WHERE id=?` (.Req.PathValue "id")}}
<div>
  <span>Name: {{$contact.name}}</span>
  <span>Phone: {{$contact.phone}}</span>
</div>
{{end}}

{{define "DELETE /contact/{id}"}}
{{$_ := .DB.Exec `DELETE FROM contacts WHERE id=?` (.Req.PathValue "id")}}
{{.Resp.SetStatus 204}}
{{end}}
```

## Early return

An early return stops template execution successfully (not as an error). Triggers include:

- the `return` function: `{{return}}`
- response helpers such as `.Resp.ReturnStatus`
- some `.Flush` helper execution paths when the request or server context is cancelled (`Sleep`, `WaitForServerStop`) so streams can stop cleanly

Handlers treat the internal sentinel as normal completion (not a failure). Use `failf` when you want a real failure:

```html
{{if eq (.Req.FormValue "name") ""}}{{failf "name is required"}}{{end}}
```

## Related

- [Instance loading](instance-loading.md)
- [Glossary](glossary.md) — path template, define template, early return
