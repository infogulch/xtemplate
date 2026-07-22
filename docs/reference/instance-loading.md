# Instance loading

How an instance is built from a template root: walking files, parsing templates, hashing static files, and registering routes. An instance is immutable after load; a reload builds a new instance and swaps it in.

## Walking the root directory

While an instance is loading, xtemplate walks the private build-root FS (`templatesFS`). It comes from `Source.Start` (or `WithTemplateFS` / `WithTemplateDir`). The default source is `os` with path `templates`.

- Files matching `Config.TemplateExtension` (default `.html`) are parsed into the instance's template namespace: each becomes a path template (and may define additional define templates).
- All other files are static files (served as-is), except compressed siblings of static files (`.gz`, `.zst`, `.br`) which become alternate encodings of the identity file.
- Hidden **directories** (names starting with `.`, e.g. `.git`) are skipped when walking, so their contents are not loaded.
- Hidden **file basenames** (e.g. `.env`, `.htaccess`) under a normal directory are still loaded: non-templates become static routes; path templates are parsed into the global namespace (so you can `{{template "/shared/.head.html" .}}`) but are not given a file-based GET route.

## Path templates (from disk)

Each matching file under the template root is read once, optionally minified, and parsed into the instance’s global template namespace.

### Minification

When `minify` is true (the default), HTML sources for path templates are minified at load time with [tdewolff/minify](https://github.com/tdewolff/minify) before parse. This is a build-time transform of the template source, not a per-response filter.

### Initialization templates

Any template whose name begins with `INIT ` (note the trailing space) is an initialization template. After the instance is fully built each initialization template is executed once with a synthetic request and the **buffered** dot context (`.Resp` is available; `.Flush` is not). Its output is discarded. An error fails the load (or reload), so the previous instance stays live if this was a reload.

Typical use: seed schema.

```html
{{define "INIT contacts-setup"}}
{{$_ := .DB.Exec `CREATE TABLE IF NOT EXISTS contacts (
  id INTEGER PRIMARY KEY, name TEXT NOT NULL, phone TEXT)`}}
{{end}}
```

## Static files

Static files are read at startup to calculate their content checksums, which are cached in memory so templates can emit content-addressed, SRI-enabled URLs from a single lookup (`.X.StaticFileHash`).

Serving uses the real filesystem when available (including `sendfile`-style optimizations via the Go standard library). Clients can negotiate compressed encodings when alternate files exist.

### Precompressed static files

If siblings of a static file exist with extensions `.gz`, `.zst`, or `.br`, they are treated as pre-compressed encodings of the identity file. At load time xtemplate decompresses them and checks that the content hash matches the identity; a mismatch is a load error. At request time, content negotiation selects an encoding the client accepts.

Example layout:

```
templates/assets/reset.css
templates/assets/reset.css.gz
templates/assets/reset.css.br
```

### Precompression during initialization

`Config.Precompress` (CLI: `--precompress`, repeatable) can generate compressed variants at load time for encodings `gzip`, `zstd`, and `br`. Existing precompressed siblings are left alone. Useful when you do not want to commit compressed static files to the repo.

## Routing

Routes are registered on an `http.ServeMux` using Go 1.22+ patterns.

### File-based routing

For a path template (the whole file is the template named by its path):

1. Extension is stripped (`TemplateExtension`).
2. Files named `index` or `index{$}` handle the directory (canonical trailing slash URL).
3. Hidden basenames (leading `.`) get no route.
4. Pattern is `GET` + cleaned path.

```
File path:              ServeMux pattern:
.
├── index.html          GET /{$}   (i.e. GET /)
├── todos.html          GET /todos
├── admin
│   └── settings.html   GET /admin/settings
├── posts
│   └── {slug}.html     GET /posts/{slug}
└── shared
    └── .head.html      (loaded, not routed)
```

Path placeholders in file names become ServeMux wildcards and are available as `.Req.PathValue`.

### Define-based routing

A `{{define}}` whose name matches `METHOD path` registers that route:

| Name prefix | Handler kind |
|---|---|
| `GET`, `POST`, `PUT`, `PATCH`, `DELETE` | Buffered template handler (`.Resp` available) |
| `ANY` | Buffered handler registered with no method (matches every HTTP method). Path `/` is a ServeMux subtree matching every path. |
| `SSE` | Flushing handler (`.Flush` available); registered as `GET` for the path. Clients must send `Accept: text/event-stream` (browser `EventSource` does) or the request is rejected with `406`. |

```html
{{define "GET /contact/{id}"}}
  {{$c := .DB.QueryRow `SELECT name, phone FROM contacts WHERE id=?` (.Req.PathValue "id")}}
  <div>{{$c.name}} - {{$c.phone}}</div>
{{end}}

{{define "DELETE /contact/{id}"}}
  {{$_ := .DB.Exec `DELETE FROM contacts WHERE id=?` (.Req.PathValue "id")}}
  {{.Resp.SetStatus 204}}
{{end}}

{{define "SSE /live-updates"}}
  {{/* stream with .Flush */}}
{{end}}
```

These named defines are used for routing, but they are also available for ordinary template invocation.

Define names that are not routes (e.g. `navbar`) are ordinary templates invocable with `{{template "navbar" .}}`.

## Related

- [Template semantics](template-semantics.md) - execution and invocation
- [Dot context](dot-context.md) - per-request `.`
- [Design](../explanation/design.md) - why this model
