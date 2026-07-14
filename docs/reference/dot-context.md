# Dot context

The dot context is the value of `.` on every template execution for a request. It is a struct assembled per request from builtin providers plus any configured core or custom [dot providers](glossary.md#providers).

It is the sole channel for request data, response control, and backing data sources. Stateless helpers are template functions ([FuncMap](functions.md)), not request-scoped.

## Builtin providers

| Field | Available on | Go docs |
|---|---|---|
| `.X` | all requests | [DotX](https://pkg.go.dev/github.com/infogulch/xtemplate#DotX) |
| `.Req` | all requests | [DotReq](https://pkg.go.dev/github.com/infogulch/xtemplate#DotReq) |
| `.Resp` | buffered handlers | [DotResp](https://pkg.go.dev/github.com/infogulch/xtemplate#DotResp) |
| `.Flush` | flushing / SSE handlers | [DotFlush](https://pkg.go.dev/github.com/infogulch/xtemplate#DotFlush) |

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

### Response control in `.Resp`

Available on buffered template handlers (normal `GET`/`POST`/… routes). Output is buffered so status and headers can be set during execution; on error the buffer is discarded.

Common methods: `AddHeader`, `SetHeader`, `DelHeader`, `SetStatus`, `ReturnStatus` (status + early return), `ServeContent`. Full API: [DotResp](https://pkg.go.dev/github.com/infogulch/xtemplate#DotResp).

```html
{{.Resp.AddHeader "Location" "/"}}
{{.Resp.ReturnStatus 303}}
```

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

Methods include `QueryRows`, `QueryRow`, `QueryVal`, `Exec`, and explicit `Commit`. Each request gets a value that opens a transaction on first use and commits on success or rolls back on error (via `CleanupDotProvider`).

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

### Messaging with `nats`

Package: [`providers/dotnats`](https://pkg.go.dev/github.com/infogulch/xtemplate/providers/dotnats). Provider type: `"nats"`. Send, request/reply, and stream-oriented patterns. Integration tests under [`test/templates/nats/`](../../test/templates/nats/) exercise a working configuration with an in-process server.

For raw JSON provider config, nats connection options follow the [`nats.Options`](https://pkg.go.dev/github.com/nats-io/nats.go#Options) field names (e.g. `"Url"` with a capital `U` when setting the server URL). Caddyfile `conn_options { url … }` maps correctly via the Caddyfile adapter.

### Email with `smtp`

Package: [`providers/dotsmtp`](https://pkg.go.dev/github.com/infogulch/xtemplate/providers/dotsmtp). Provider type: `"smtp"`. Synchronous, send-only SMTP delivery. Body rendering stays with [`.X.Template`](#instance-data-in-x); this provider only transports already-rendered strings. There is no built-in queue — compose with `nats`/JetStream if you need durable async delivery.

Typical field name: `.Email`. Config requires `host` and `from` (default sender). Optional connection settings: `port` (default 587), `username` / `password`, `auth` (`plain`, `login`, `cram-md5`, `none`, or empty for auto), `tls` (`starttls` default, `tls`, `none`), `helo`. Safety limits: `max_recipients` (default 50), `max_message_bytes` (default 1 MiB), `send_timeout` (default 30s; JSON is nanoseconds — see [Configuration](configuration.md#provider-types)).

```html
{{$body := .X.Template "email/welcome.html" .}}
{{$id := .Email.Send "alice@example.com" "Welcome!" $body}}
```

`Send(to, subject, body, extra…)` delivers one message and returns the generated Message-ID. `to` is a single address string or a list of address strings (each string is one RFC 5322 address; recipients are not split on commas). Optional final map keys: `cc`, `bcc` (same shape as `to`), `from` (override default sender), `replyTo`, `text` (plaintext alternative). Unknown keys and wrong value types error.

## Custom providers

Implement `xtemplate.DotConfig` and attach with `WithProvider` (or register a provider type for JSON/Caddyfile). See [How to create a custom dot provider](../how-to/create-a-provider.md) and [`examples/dotprovider`](../../examples/dotprovider/).
