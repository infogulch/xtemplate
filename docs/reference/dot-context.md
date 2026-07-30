# Dot context

The dot context is the value of `.` on every template execution for a request. It is a struct assembled per request from builtin providers plus any configured core or custom [dot providers](glossary.md#providers).

It is the sole channel for request data, response control, and backing data sources. Stateless helpers are template functions ([FuncMap](functions.md)), not request-scoped.

## Builtin providers

| Field | Available on | Go docs |
|---|---|---|
| `.X` | all requests | [DotX](https://pkg.go.dev/github.com/infogulch/xtemplate#DotX) |
| `.Req` | all requests | [DotReq](https://pkg.go.dev/github.com/infogulch/xtemplate#DotReq) |
| `.Vars` | all requests | [DotVars](https://pkg.go.dev/github.com/infogulch/xtemplate#DotVars) |
| `.Resp` | buffered handlers | [DotResp](https://pkg.go.dev/github.com/infogulch/xtemplate#DotResp) |
| `.Flush` | flushing / SSE handlers | [DotFlush](https://pkg.go.dev/github.com/infogulch/xtemplate#DotFlush) |

Field assembly order is `{X, Req, Vars}` + configured providers + `{Resp | Flush}`. The names `X`, `Req`, `Vars`, `Resp`, and `Flush` are reserved.

### Instance data in `.X`

Read-only view of the loaded instance. Common methods:

| Method | Role |
|---|---|
| `StaticFileHash path` | Content hash for SRI / cache-busting query params |
| `Template name dot` | Execute the named template with the given dot; returns rendered HTML |
| `Func name` | Look up a template function by name |

Full API: [DotX](https://pkg.go.dev/github.com/infogulch/xtemplate#DotX).

```html
{{- with $hash := .X.StaticFileHash `/assets/reset.css`}}
<link rel="stylesheet" href="/assets/reset.css?hash={{$hash}}" integrity="{{$hash}}">
{{- end}}
```

When the query string carries the content hash, xtemplate can emit long-lived `Cache-Control` so clients cache aggressively without stale static files after a change.

### Request details in `.Req`

Embeds `*http.Request`. Use standard fields and methods: `Method`, `URL`, `Header`, `PathValue`, `FormValue`, `Cookie`, and so on.

Call `.Req.ParseForm` before relying on `.Req.Form` / `.Req.PostForm` if you are not using `FormValue` (which parses as needed).

```html
<p>Path: {{.Req.URL.Path}}</p>
<p>Id: {{.Req.PathValue "id"}}</p>
<p>Name: {{.Req.FormValue "name"}}</p>
```

### Request scratch in `.Vars`

Per-request `map[string]any` for light composition: define-templates can act as shallow subroutines with out-params. A **non-nil empty map** is created every request; maps are not shared across requests and are not a session or flash store.

Primary API:

| Method | Role |
|---|---|
| `Set key value` | Store a value; returns `""` |
| `Get key` | Value or **nil** if missing |
| `Has key` | Whether the key is present |
| `Delete key` | Remove a key; returns `""` |

Keys must be non-empty strings. Prefer short noun keys (`list`, `todo`, `owner`) and set-then-get in the same handler tree. If a helper needs more than a few keys or real domain logic, use a custom provider or FuncMap instead.

```html
{{define "require-list"}}
  {{- $r := try .DB "QueryRow" `SELECT id, name FROM lists WHERE id=?` (.Req.PathValue "id")}}
  {{- if not $r.OK}}
    {{.Resp.RespondWith 404 (.X.Template "/shared/.404.html" .)}}
  {{- end}}
  {{- .Vars.Set "list" $r.Value -}}
{{end}}

{{define "POST /list/{id}/todos"}}
  {{- template "require-list" .}}
  {{- $list := .Vars.Get "list"}}
  {{- /* mutate using $list */}}
{{end}}
```

Because `.Vars` is a map, Sprig dict helpers (`set`, `index`, `hasKey`, `range`) also work. Prefer the method API in app code. **Caveat:** Sprig `get` returns `""` on missing keys; `.Vars.Get` returns `nil`.

Full API: [DotVars](https://pkg.go.dev/github.com/infogulch/xtemplate#DotVars).

### Response control in `.Resp`

Available on buffered template handlers (normal `GET`/`POST`/… routes). Output is buffered so status and headers can be set during execution; on error the buffer is discarded.

Common methods: `AddHeader`, `SetHeader`, `DelHeader`, `SetStatus`, `ReturnStatus` (status + early return, **keeps** buffer), `RespondWith` (status + body, **replaces** buffer), `ServeContent` (file-like payload, replaces buffer). Full API: [DotResp](https://pkg.go.dev/github.com/infogulch/xtemplate#DotResp).

```html
{{.Resp.AddHeader "Location" "/"}}
{{.Resp.ReturnStatus 303}}
```

#### Replace the response with `RespondWith`

Use when discovery mid-handler shows the client should not see partial template output: 404 pages, plain-text 400s, empty-body redirects.

```html
{{.Resp.AddHeader "Location" "/"}}
{{.Resp.RespondWith 303 ""}}

{{.Resp.RespondWith 400 "name is required"}}

{{.Resp.RespondWith 404 (.X.Template "/shared/.404.html" .)}}
```

| Exit | Buffer | Body |
|---|---|---|
| Success / `return` / `ReturnStatus` | **kept** | whatever the template wrote |
| **`RespondWith`** | **replaced** | explicit body (`""` when empty) |
| `ServeContent` | replaced | file-like content |
| `failf` | discarded | generic 500 |

- Body is required; pass `""` for empty. Supported types: `string`, `template.HTML`, `[]byte`.
- Headers set on `.Resp` before `RespondWith` are kept.
- Prefer `.X.Template` for HTML pages (private buffer → `template.HTML`), then install with `RespondWith`. Do not `{{template "404" .}}` then `ReturnStatus` for the replace case — that appends into the request buffer.
- If body is `template.HTML`, non-empty, and `Content-Type` is unset, it defaults to `text/html; charset=utf-8`. Empty bodies do not force a `Content-Type`.
- `RespondWith` is buffered-only (field is `.Resp`); not available on `.Flush` / SSE.

### Streaming control in `.Flush`

Available on flushing handlers - routes defined with the `SSE` method prefix. Use for Server-Sent Events and other incremental responses. Common methods: `SendSSE`, `Flush`, `Repeat`, `Sleep` (returns early if the request or server is cancelled), `WaitForServerStop`. Full API: [DotFlush](https://pkg.go.dev/github.com/infogulch/xtemplate#DotFlush); example: [`sse-chat`](../../examples/sse-chat/).

```html
{{- define "SSE /reload"}}{{.Flush.WaitForServerStop}}data: reload{{printf "\n\n"}}{{end}}
<script>new EventSource("/reload").onmessage = () => location.reload()</script>
```

## Core providers (configured dot fields)

Core providers are provider packages under `github.com/infogulch/xtemplate/providers/…`. Default CLI binaries and the Caddy standard provider set blank-import them. Each can contribute a dot field when configured by the user (`name` in JSON / field token in Caddyfile); examples use conventional names (`.DB`, `.FS`, …), not fixed globals.

Configure via [JSON](configuration.md#json) provider config, Caddyfile, or Go (`WithProvider` / package helpers).

### Queries and exec with `sql`

Package: [`providers/dotsql`](https://pkg.go.dev/github.com/infogulch/xtemplate/providers/dotsql). Provider type: `"sql"`.

```html
<ul>
  {{range .DB.QueryRows `SELECT id, name FROM contacts`}}
  <li><a href="/contact/{{.id}}">{{.name}}</a></li>
  {{end}}
</ul>
```

Methods include `QueryRows`, `QueryRow`, `QueryVal`, `Exec`, and explicit `Commit`. Each request gets a value that opens a transaction on first use and commits on success or rolls back on error (via `Finalizer`).

Default builds include the `sqlite3` driver ([ncruces/go-sqlite3](https://github.com/ncruces/go-sqlite3)); other drivers need a [custom build](../how-to/custom-build.md).

See also [`examples/contacts`](../../examples/contacts/).

### Filesystem access with `fs`

Package: [`providers/dotfs`](https://pkg.go.dev/github.com/infogulch/xtemplate/providers/dotfs). Provider type: `"fs"`.

```html
<ol>
{{range .FS.ReadDir "dir/"}}
  <li>{{.Name}}</li>
{{end}}
</ol>
```

Optional `writable: true` exposes multipart upload (`ReceiveFiles`). Full API: package docs. Demo: [`examples/filebrowser`](../../examples/filebrowser/).

### Static key/value config with `flags`

Package: [`providers/dotflags`](https://pkg.go.dev/github.com/infogulch/xtemplate/providers/dotflags). Provider type: `"flags"`. Exposes a fixed map of strings (feature flags, env labels, versions) without a separate config file format inside templates.

### In-process broadcast with `bus`

Package: [`providers/dotbus`](https://pkg.go.dev/github.com/infogulch/xtemplate/providers/dotbus). Provider type: `"bus"`. Process-local multi-producer multi-consumer topic fan-out for SSE and live UI — no external broker.

Typical field name: `.Bus`. Optional `buffer` is the per-subscriber channel capacity (default 16). Publish is best-effort: if a subscriber's buffer is full, that message is dropped for that subscriber (never blocks the publisher). Prefer `nats` when you need multi-process delivery, request/reply, or durable streams.

```html
{{define "SSE /events"}}{{range .Bus.Subscribe "messages"}}{{$.Flush.SendSSE "" .}}{{end}}{{end}}
{{define "POST /messages"}}{{.Req.ParseForm}}{{.Bus.Publish "messages" (.Req.FormValue "msg")}}ok{{end}}
```

JSON:

```json
{ "type": "bus", "name": "Bus", "buffer": 16 }
```

### Messaging with `nats`

Package: [`providers/dotnats`](https://pkg.go.dev/github.com/infogulch/xtemplate/providers/dotnats). Provider type: `"nats"`. Send, request/reply, and stream-oriented patterns. Integration tests under [`test/templates/nats/`](../../test/templates/nats/) exercise a working configuration with an in-process server.

For raw JSON provider config, nats connection options follow the [`nats.Options`](https://pkg.go.dev/github.com/nats-io/nats.go#Options) field names (e.g. `"Url"` with a capital `U` when setting the server URL). Caddyfile `conn_options { url … }` maps correctly via the Caddyfile adapter.

### Email with `smtp`

Package: [`providers/dotsmtp`](https://pkg.go.dev/github.com/infogulch/xtemplate/providers/dotsmtp). Provider type: `"smtp"`. Synchronous, send-only SMTP delivery. Body rendering stays with [`.X.Template`](#instance-data-in-x); this provider only transports already-rendered strings. There is no built-in queue — compose with `nats`/JetStream if you need durable async delivery.

Typical field name: `.Email`. Config requires `host` and `from` (default sender). Optional connection settings: `port` (default 587), `username` / `password`, `auth` (`plain`, `login`, `cram-md5`, `none`, or empty for auto), `tls` (`starttls` default, `tls`, `none`), `helo`. Safety limits: `max_recipients` (default 50), `max_message_bytes` (default 1 MiB), `send_timeout` (default `"30s"`; JSON duration **string** only — see [Configuration](configuration.md#provider-types)).

```html
{{$body := .X.Template "email/welcome.html" .}}
{{$id := .Email.Send "alice@example.com" "Welcome!" $body}}
```

`Send(to, subject, body, extra…)` delivers one message and returns the generated Message-ID. `to` is a single address string or a list of address strings (each string is one RFC 5322 address; recipients are not split on commas). Optional final map keys: `cc`, `bcc` (same shape as `to`), `from` (override default sender), `replyTo`, `text` (plaintext alternative). Unknown keys and wrong value types error.

## Custom providers

Implement `xtemplate.Provider` and attach with a `WithProvider` factory (or register a provider type for JSON/Caddyfile). See [How to create a custom dot provider](../how-to/create-a-provider.md) and [`examples/dotprovider`](../../examples/dotprovider/).
