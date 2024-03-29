# xtemplate

`xtemplate` is a html/template-based hypertext preprocessor and rapid
application development web server written in Go. It streamlines construction of
hypermedia-exchange-oriented web sites by efficiently handling basic server
tasks, enabling authors to focus on defining routes and responding to them using
templates and configurable data sources.

## 🎯 Goal

After bulding some sites with [htmx](https://htmx.org) and Go, I wished that
everything would just get out of the way of the fundamentals:

- URLs and path patterns
- Access to a backing data source
- Executing a template to return HTML

🎇 **The idea of `xtemplate` is that *templates* can be the nexus of these
fundamentals.**

<details><summary>🚫 Anti-goals</summary>

`xtemplate` needs to implement some of the things that are required to make a
good web server in a way that avoids common issues with existing web server
designs, otherwise they'll be in the way of the fundamentals:

* **Rigid template behavior**: Most designs relegate templates to be dumb string
  concatenators with just enough dynamic behavior to walk over some fixed data
  structure.
* **Inefficient template loading**: Some designs read template files from disk
  and parse them on every request. This seems wasteful when the web server
  definition is typically static.
* **Constant rebuilds**: On the other end of the spectrum, some designs require
  rebuilding the entire server from source when any little thing changes. This
  seems wasteful and makes graceful restarts more difficult than necessary when
  all you're doing is changing a button name.
* **Repetitive route definitions**: Why should you have to name a http handler
  and add it to a central registry (or maintain a pile of code that plumbs these
  together for you) when new routes are often only relevant to the local html?
* **Default unsafe**: Some designs require authors to vigilantly escape user
  inputs, risking XSS attacks that could have been avoided with less effort.
* **Inefficient asset serving**: Some designs compress static assets at request
  time, instead of serving pre-compressed content with sendfile(2) and
  negotiated content encoding. Most designs don't give templates access to the
  hash of asset files, depriving clients of enough information to optimize
  caching behavior and check resource integrity.

</details>

## ✨ Features

*Click a feature to expand and show details:*

<details open><summary><strong>⚡ Efficient design</strong></summary>

> All template files are read and parsed *once*, at startup, and kept in memory
> during the life of an xtemplate *instance*. Requests are routed to a handler
> that immediately starts executing a template reference in response. No slow
> cascading disk accesses or parsing overhead before you even begin crafting the
> response.
</details>

<details><summary><strong>🔄 Live reload</strong></summary>

> Template files are loaded into a new instance and validated milliseconds after
> they are modified, no need to restart the server. If an error occurs during
> load the previous instance remains intact and continues to serve while the
> loading error is printed to the logs. A successful reload atomically swaps the
> handler so new requests are served by the new instance; pending requests are
> allowed to complete gracefully.
>
> Add this template definition and one-line script to your page, then
> clients will automatically reload when the server does:
>
> ```html
> {{- define "SSE /reload"}}{{.WaitForServerStop}}data: reload{{printf "\n\n"}}{{end}}
> <script>new EventSource("/reload").onmessage = () => location.reload()</script>
> <!-- Maybe not a great idea for production, but you do you. -->
> ```
</details>

<details open><summary><strong>🗃️ Simple file-based routing</strong></summary>

> `GET` requests are handled by invoking a matching template file at that path.
> (Hidden files that start with `.` are loaded but not routed by default.)
>
> ```
> File path:              HTTP path:
> .
> ├── index.html          GET /
> ├── todos.html          GET /todos
> ├── admin
> │   └── settings.html   GET /admin/settings
> └── shared
>     └── .head.html      (not routed because it starts with '.')
> ```
</details>

<details><summary><strong>🔱 Add custom routes to handle any method and path pattern</strong></summary>

> Handle any [Go 1.22 ServeMux](servemux) pattern by **defining a template with
> that pattern as its name**. Path placeholders are available during template
> execution with the `.Req.PathValue` method.
>
> ```html
> <!-- match on path parameters -->
> {{define "GET /contact/{id}"}}
> {{$contact := .QueryRow `SELECT name,phone FROM contacts WHERE id=?` (.Req.PathValue "id")}}
> <div>
>   <span>Name: {{$contact.name}}</span>
>   <span>Phone: {{$contact.phone}}</span>
> </div>
> {{end}}
>
> <!-- match on any http method -->
> {{define "DELETE /contact/{id}"}}
> {{$_ := .Exec `DELETE from contacts WHERE id=?` (.Req.PathValue "id")}}
> {{.RespStatus 204}}
> {{end}}
> ```

[servemux]: https://tip.golang.org/doc/go1.22#enhanced_routing_patterns

</details>

<details><summary><strong>👨‍💻 Define and invoke custom templates</strong></summary>

> All html files under the template root directory are available to invoke by
> their full path relative to the template root dir starting with `/`:
>
> ```html
> <html>
>   <title>Home</title>
>   <!-- import the contents of another file -->
>   {{template "/shared/.head.html" .}}
>
>   <body>
>     <!-- invoke a custom named template defined anywhere -->
>     {{template "navbar" .}}
>     ...
>   </body>
> </html>
> ```
</details>

<details><summary><strong>🛡️ XSS safe by default</strong></summary>

> The html/template library automatically escapes user content, so you can rest
> easy from basic XSS attacks. The defacto standard html sanitizer for Go,
> BlueMonday, is available for cases where you need finer grained control.
>
> If you have some html string that you do trust, it's easy to inject if that's
> your intention with the `trustHtml` func.
</details>

<details open><summary><strong>🎨 Customize the context to provide selected data sources</strong></summary>

> Configure xtemplate to get access to built-in and custom data sources like
> running SQL queries against a database, sending and receiving messages using a
> message streaming client like NATS, read and list files from a local
> directory, reading static config from a key-value store, **or perform any
> action you can define by writing a Go API**, like the common "repository"
> design pattern for example.
>
> Modify `Config` to add built-in or custom `ContextProvider` implementations,
> and they will be made available in the dot context.
>
> Some built-in context providers are listed next:
</details>

<details><summary><strong>💽 Database context provider: Execute queries</strong></summary>

> Add the built-in Database Context Provider to run queries using the configured
> Go driver and connection string for your database. (Supports the `sqlite3`
> driver by default, compile with your desired driver to use it.)
>
> ```html
> <ul>
>   {{range .Tx.Query `SELECT id,name FROM contacts`}}
>   <li><a href="/contact/{{.id}}">{{.name}}</a></li>
>   {{end}}
> </ul>
> ```
</details>

<details><summary><strong>🗄️ Filesystem context provider: List and read local files</strong></summary>

> Add the built-in Filesystem Context Provider to List and read
> files from the configured directory.
>
> ```html
> <p>Here are the files:
> <ol>
> {{range .ListFiles "dir/"}}
>   <li>{{.Name}}</li>
> {{end}}
> </ol>
> ```
</details>

<details><summary><strong>💬 NATS context provider: Send and receive messages</strong></summary>

> Add and configure the NATS Context Provider to send messages, use the
> Request-Response pattern, and even send live updates to a client.
>
> ```html
> <example></example>
> ```
</details>

<details open><summary><strong>📤 Optimal asset serving</strong></summary>

> Non-template files in the templates directory are served directly from disk
> with appropriate caching responses, negotiating with the client to serve
> compressed versions. Efficient access to the content hash is available to
> templates for efficient SRI and perfect cache behavior.
>
> If a static file also has .gz, .br, .zip, or .zst copies, they are decoded and
> hashed for consistency on startup, and use the `Accept-Encoding` header to
> negotiate an appropriate `Content-Encoding` with the client and served
> directly from disk.
>
> Templates can efficiently access the static file's precalculated content hash
> to build a `<script>` or `<link>` integrity attribute, instructing clients to
> check the integrity of the content if they are served through a CDN. See:
> [Subresource Integrity](https://developer.mozilla.org/en-US/docs/Web/Security/Subresource_Integrity)
>
> Add the content hash as a query parameter and responses will automatically add
> a 1 year long `Cache-Control` header so clients can safely cache as long as
> possible. If the file changes, its hash and thus query parameter will change
> so the client will immediately request a new version, **entirely eliminating
> stale cache issues**.
>
> ```html
> {{- with $hash := .StaticFileHash `/reset.css`}}
> <link rel="stylesheet" href="/reset.css?hash={{$hash}}" integrity="{{$hash}}">
> {{- end}}
> ```
</details>

<details><summary><strong>📬 Live updates with Server Sent Events (SSE)</strong></summary>

> Define a template with a name that starts with SSE, like `SSE /url/path`, and
> SSE requests will be handled by invoking the template. Individual messages can
> be sent by using `.Flush`, and the template can be paused to wait on messages
> sent over Go channels or can block on server shutdown.
</details>

<details><summary><strong>🐜 Small footprint and easy deployment</strong></summary>

> Compiles to a ~30MB binary. Easily add your own custom functions and choice of
> database driver on top. Deploy next to your templates and static files or
> [embed](https://pkg.go.dev/embed) them into the binary for single binary
> deployments.
</details>

## 📦 How to run

### 1. 📦 As a Caddy plugin

The `xtemplate-caddy` plugin offers all `xtemplate` features integrated into
[Caddy](https://caddyserver.com), a fast and extensible multi-platform
HTTP/1-2-3 web server with automatic HTTPS.

Download Caddy with `xtemplate-caddy` middleware plugin built-in:

https://caddyserver.com/download?package=github.com%2Finfogulch%2Fxtemplate-caddy

This is the simplest Caddyfile that uses the `xtemplate-caddy` plugin:

```Caddyfile
routes {
  xtemplate
}
```

### 2. 📦 As the default CLI application

Download from the [Releases page](https://github.com/infogulch/xtemplate/releases) or build the binary in [`./cmd`](./cmd/).

Custom builds can include your chosen db drivers, make Go functions available to
the template definitions, and even embed templates for true single binary
deployments. The [`./cmd` package](./cmd/) is the reference CLI application,
consider starting your customization there.

<details><summary><strong>🎏 CLI flags and examples: (click to show)</strong></summary>

```shell
$ ./xtemplate -help
xtemplate is a hypertext preprocessor and html templating http server

Usage: ./xtemplate [options]

Options:
  -listen string              Listen address (default "0.0.0.0:8080")

  -template-path string       Directory where templates are loaded from (default "templates")
  -watch-template bool        Watch the template directory and reload if changed (default true)
  -template-extension string  File extension to look for to identify templates (default ".html")
  -minify bool                Preprocess the template files to minimize their size at load time (default false)
  -ldelim string              Left template delimiter (default "{{")
  -rdelim string              Right template delimiter (default "}}")

  -context-path string        Directory that template definitions are given direct access to. No access is given if empty (default "")
  -watch-context bool         Watch the context directory and reload if changed (default false)

  -db-driver string           Name of the database driver registered as a Go 'sql.Driver'. Not available if empty. (default "")
  -db-connstr string          Database connection string

  -c string                   Config values, in the form 'x=y'. Can be used multiple times

  -log int                    Log level. Log statements below this value are omitted from log output, DEBUG=-4, INFO=0, WARN=4, ERROR=8 (Default: 0)
  -help                       Display help

Examples:
    Listen on port 80:
    $ ./xtemplate -listen :80

    Specify a context directory and reload when it changes:
    $ ./xtemplate -context-path context/ -watch-context

    Parse template files matching a custom extension and minify them:
    $ ./xtemplate -template-extension ".go.html" -minify

    Open the specified db and makes it available to template files as '.DB':
    $ ./xtemplate -db-driver sqlite3 -db-connstr 'file:rss.sqlite?_journal=WAL'
```
</details>

### 3. 📦 As a Go library

[![Go Reference](https://pkg.go.dev/badge/github.com/infogulch/xtemplate.svg)](https://pkg.go.dev/github.com/infogulch/xtemplate)

xtemplate's public Go API starts with a [`xtemplate.Config`](./config.go),
from which you can get either an [`xtemplate.Instance`](./instance.go) interface
or a [`xtemplate.Server`](./server.go) interface, with the methods
`config.Instance()` and `config.Server()`, respectively.

An `xtemplate.Instance` is an immutable `http.Handler` that can handle requests,
and exposes some metadata about the files loaded as well as the ServeMux
patterns and associated handlers for individual routes. An `xtemplate.Server`
also handles http requests by forwarding requests to an internal Instance, but
the `Server` can be reloaded by calling `server.Reload()`, which creates a new
Instance with the previous config and atomically switches the handler to direct
new requests to the new Instance.

Use an Instance if you have no interest in reloading, or if you want to use
xtemplate handlers in your own mux. Use a Server if you want an easy way to
smoothly reload and replace the xtemplate Instance behind a http.Handler at
runtime.

## 👨‍🏭 How to use

### 🧰 Template semantics

`xtemplate` templates are based on Go's `html/template` package, with some
additional features and enhancements. Here are the key things to keep in mind:

- All template files are loaded recursively from the specified root directory,
  and they are parsed and cached in memory at startup.
- Each template file is associated with a specific route based on its file path.
  For example, `index.html` in the root directory will handle requests to the
  `/` path, while `admin/settings.html` will handle requests to
  `/admin/settings`.
- You can define custom routes by defining a template with a special name in
  your template files. For example, `{{define "GET /custom-route"}}...{{end}}`
  will create a new route that handles GET requests to `/custom-route`. Names
  also support path parameters as defined by [http.ServeMux](servemux).
- Template files can be invoked from within other templates using either their
  full path relative to the template root or by using its defined template name.
- Templates are executed with a uniform context object, which provides access to
  request data, database connections, and other useful dynamic functionality.
- Templates can also call functions set at startup.

[servemux]: https://pkg.go.dev/net/http#ServeMux

> [!note]
>
> Custom dot fields and functions are similar in that they both add
> functionality to the templates, but dot fields are distinguished in that they
> are initialized on every request with access to request-scoped details
> including the underlying `http.Request` and `http.ResponseWriter` objects, the
> request-scoped logger, and the server context.
>
> Thus FuncMap functions are recommended for adding simple computational
> functionality (like parsing, escaping, data structure manipulation, etc),
> whereas dot fields are recommended for more complicated tasks like accessing
> network resources, running database queries, accessing the file system, etc.

### 📝 Context

The dot context `{{.}}` set on each template invocation provides access to
request-specific data and response control methods, and can be modified to add
custom fields with your own methods.

#### ✏️ Built-in dot fields

These fields are always present in relevant template invocations:

* Access instance data with the `.X` field. See [DotX]
* Access request details with the `.Req` field. See [DotReq]
* Control the HTTP response in buffered template handlers with the `.Resp`
  field. See [DotResp]
* Control flushing behavior for flushing template handlers (i.e. SSE) with the
  `.Flush` field. See [DotFlush]

[DotX]: https://pkg.go.dev/github.com/infogulch/xtemplate#DotX
[DotReq]: https://pkg.go.dev/github.com/infogulch/xtemplate#DotReq
[DotResp]: https://pkg.go.dev/github.com/infogulch/xtemplate#DotResp
[DotFlush]: https://pkg.go.dev/github.com/infogulch/xtemplate#DotFlush

#### ✏️ Optional dot fields

These optional value providers can be configured with any field name, and can be
configured multiple times with different configurations.

* Read and list files. See [DotFS]
* Query and execute SQL statements. See [DotDB]
* Read template-level key-value map. See [DotKV]

[DotFS]: https://pkg.go.dev/github.com/infogulch/xtemplate/providers#DotFS
[DotDB]: https://pkg.go.dev/github.com/infogulch/xtemplate/providers#DotDB
[DotKV]: https://pkg.go.dev/github.com/infogulch/xtemplate/providers#DotKV

#### ✏️ Custom dot fields

You can create custom dot fields that expose arbitrary Go functionality to your
templates. See [👩‍⚕️ Writing a custom `DotProvider`](#-writing-a-custom-dotprovider).

### 📐 Functions

These are built-in functions that are available to all invocations and don't
depend on request context or mutate state. There are three sets by default:
functions that come by default in the go template library, functions from the
sprig library, and custom functions added by xtemplate.

You can custom FuncMaps by configuring the `Config.FuncMaps` field.

* 📏 `xtemplate` includes funcs to render markdown, sanitize html, convert
  values to human-readable forms, and to try to call a function to handle an
  error within the template. See the free functions named [`FuncXYZ(...)` in
  xtemplate's Go docs](funcgodoc) for details.
* 📏 Sprig publishes a library of useful template funcs that enable templates to
  manipulate strings, integers, floating point numbers, and dates, as well as
  perform encoding tasks, manipulate lists and dicts, converting types,
  and manipulate file paths See [Sprig Function Documentation](sprig).
* 📏 Go's built in functions add logic and basic printing functionality.
  See: [text/template#Functions](gofuncs).

[sprig]: https://masterminds.github.io/sprig/
[gofuncs]: https://pkg.go.dev/text/template#hdr-Functions
[funcgodoc]: https://pkg.go.dev/github.com/infogulch/xtemplate#FuncHumanize

## 🏆 Users

* [infogulch/xrss](https://github.com/infogulch/xrss), an rss feed reader built with htmx and inline css.
* [infogulch/todos](https://github.com/infogulch/todos), a demo todomvc application.

## 👷‍♀️ Development

### 🗺️ Repository structure

xtemplate is split into the following packages:

* `github.com/infogulch/xtemplate`, a library that loads template files and
  implements an `http.Handler` that routes requests to templates and serves
  static files.
* `github.com/infogulch/xtemplate/providers`, contains optional dot provider
  implementations for common functionality.
* `github.com/infogulch/xtemplate/cmd`, a simple binary that configures
  `xtemplate` with CLI args and serves http requests with it.
* [`github.com/infogulch/xtemplate-caddy`](https://github.com/infogulch/xtemplate-caddy),
  uses xtemplate's Go library API to integrate xtemplate into Caddy server as a
  Caddy module.

> [!TIP]
>
> To understand how the xtemplate package works, it may be helpful to skim
> through the files in this order: [`config.go`](./config.go),
> [`server.go`](./server.go) [`instance.go`](./instance.go),
> [`build.go`](./build.go), [`handlers.go`](./handlers.go).

### Testing

xtemplate is tested by running [`./test/test.go`](./test/test.go) which runs
xtemplate configured to use `test/templates` as the templates dir and
`test/context` as the FS dot provider, and runs hurl files from the `test/tests`
directory.

### 👩‍⚕️ Writing a custom `DotProvider`

Implement the `xtemplate.RegisteredDotProvider` interface on your type and
register it with `xtemplate.Register()`. Optionally implement
`encoding.TextMarshaller` and `encoding.TextUnmarshaller` to round-trip
configuration from cli flags.

On startup xtemplate will create a struct that includes your value as a field.
For every request your DotProvider.Value method is called with request details
and its return value is assigned onto the struct which is passed to
`html/template` as the dot value `{{.}}`.

## ✅ Project history and license

The idea for this project started as [infogulch/go-htmx][go-htmx] (now
archived), which included the first implementations of template-name-based
routing, exposing sql db functions to templates, and a persistent templates
instance shared across requests and reloaded when template files changed.

go-htmx was refactored and rebased on top of the [templates module from the
Caddy server][caddyhttp-templates] to create `caddy-xtemplate` to add some extra
features including reading files directly and built-in funcs for markdown
conversion, and to get a jump start on supporting the broad array of web server
features without having to implement them from scratch.

xtemplate has since been refactored to be usable independently from Caddy.
Instead, [xtemplate-caddy](https://github.com/infogulch/xtemplate-caddy) is
published as a separate module that depends on the xtemplate Go API and
integrates xtemplate into Caddy as a Caddy http middleware.

`xtemplate` is licensed under the Apache 2.0 license. See [LICENSE](./LICENSE)

[go-htmx]: https://github.com/infogulch/go-htmx
[caddyhttp-templates]: https://github.com/caddyserver/caddy/tree/master/modules/caddyhttp/templates
