# xtemplate

`xtemplate` is a html/template-based hypertext preprocessor and rapid
application development web server written in Go. Its good defaults handle
typical server activity, enabling authors to focus on building
hypermedia-exchange-oriented websites by defining routes and responding to them
with template-generated HTML combined with configurable backing data stores.

## 🎯 Goal

Frustrated with the status-quo of frameworks that add more abstractions than
they solve problems, I set out to create something that feels more like
wrestling directly with first-class citizens of the web:

- HTTP paths and matching server path patterns
- Responding to an HTTP request with a template-generated HTML
- Access to various backing data sources

🎇 **The idea of `xtemplate` is that all of these can be managed with a
directory of Go *template* files.**

<details><summary>🚫 Anti-goals</summary>

`xtemplate` implements things that are required to make a good web server in a
way that avoids common pitfalls with existing engines:

- **Rigid template behavior**: Engines typically relegate templates to be dumb
string concatenators with just enough dynamic behavior to walk over some known
fixed data structure.
- **Inefficient template loading**: Many engines often load template files from
disk and parse them on *every request*, which is wasteful when web server
definitions are largely static.
- **Constant rebuilds**: Yet other engines rebuild the entire program from
source when any little thing changes.
- **Unnecessary handler names**: You've already had to name the http path and
write the associated response template, why do you have to come up with a
redundant name for the handler?
- **Default unsafe**: Some engines require authors to vigilantly escape user
inputs, risking XSS attacks that could have been avoided with less effort.
- **Inefficient asset serving**: Many engines don't try to optimize serving
assets at all and compress static assets at request time, instead of serving
pre-compressed content with sendfile(2) and negotiated content encoding. Most
designs don't give templates access to the hash of asset files, depriving
authors of the right information to optimize cache behavior and check resource
integrity.

</details>

## ✨ Features

*Click a feature to expand and show details:*

<details open><summary><strong>⚡ Efficient loading</strong></summary>

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
> {{- define "SSE /reload"}}{{.Flush.WaitForServerStop}}data: reload{{printf "\n\n"}}{{end}}
> <script>new EventSource("/reload").onmessage = () => location.reload()</script>
> <!-- Maybe not a great idea for production, but you do you. -->
> ```
>
</details>

<details open><summary><strong>🗃️ Simple file-based routing</strong></summary>

> `GET` requests are handled by invoking a matching template file at that path.
> (Hidden files that start with `.` are loaded but not routed by default.)
>
> ```ascii
> File path:              HTTP path:
> .
> ├── index.html          GET /
> ├── todos.html          GET /todos
> ├── admin
> │   └── settings.html   GET /admin/settings
> └── shared
>     └── .head.html      (not routed because it starts with '.')
> ```
>
</details>

<details><summary><strong>🔱 Add custom routes to handle any method and path pattern</strong></summary>

> Handle any [Go 1.22 ServeMux][servemux] pattern by **defining a template with
> that pattern as its name**. Path placeholders are available during template
> execution with the `.Req.PathValue` method.
>
> ```html
> <!-- match on path parameters -->
> {{define "GET /contact/{id}"}}
> {{$contact := .DB.QueryRow `SELECT name,phone FROM contacts WHERE id=?` (.Req.PathValue "id")}}
> <div>
>   <span>Name: {{$contact.name}}</span>
>   <span>Phone: {{$contact.phone}}</span>
> </div>
> {{end}}
>
> <!-- match on any http method -->
> {{define "DELETE /contact/{id}"}}
> {{$_ := .DB.Exec `DELETE from contacts WHERE id=?` (.Req.PathValue "id")}}
> {{.Resp.SetStatus 204}}
> {{end}}
> ```

[servemux]: https://tip.golang.org/doc/go1.22#enhanced_routing_patterns

</details>

<details><summary><strong>👨‍💻 Define and invoke custom templates</strong></summary>

> All html files under the template root directory are available to invoke by
> their full path relative to the template root dir starting with `/`:
>
> ```html skip_test
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
>
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
>   {{range .DB.QueryRows `SELECT id,name FROM contacts`}}
>   <li><a href="/contact/{{.id}}">{{.name}}</a></li>
>   {{end}}
> </ul>
> ```
>
</details>

<details><summary><strong>🗄️ Filesystem context provider: List and read local files</strong></summary>

> Add the built-in Filesystem Context Provider to List and read
> files from the configured directory.
>
> ```html
> <p>Here are the files:
> <ol>
> {{range .FS.ReadDir "dir/"}}
>   <li>{{.Name}}</li>
> {{end}}
> </ol>
> ```
>
</details>

<details><summary><strong>💬 NATS context provider: Send and receive messages</strong></summary>

> Add and configure the NATS Context Provider to send messages, use the
> Request-Response pattern, and even send live updates to a client.
>
> ```html
> <example></example>
> ```
>
</details>

<details open><summary><strong>📤 Optimal asset serving</strong></summary>

> Non-template files in the templates directory are served directly from disk
> with appropriate caching responses, negotiating with the client to serve
> compressed encodings if corresponding `.zst`, `.zip`, `.gz`, `.br` files are present.
>
> Templates can efficiently access static files' precalculated content hash
> to build a `<script>` or `<link>` integrity attribute, instructing clients to
> check the integrity of the content if they are served through a CDN. See:
> [Subresource Integrity (SRI)](https://developer.mozilla.org/en-US/docs/Web/Security/Subresource_Integrity)
>
> Add the content hash as a query parameter and responses will automatically add
> a 1 year long `Cache-Control` header so clients can safely cache as long as
> possible. If the file changes, its hash and thus query parameter will change
> and the client will immediately request a new version, **completely eliminating
> stale cache issues**.
>
> This example uses both SRI and precise 1-year `Cache-Control`:
>
> ```html
> {{- with $hash := .X.StaticFileHash `/assets/reset.css`}}
> <link rel="stylesheet" href="/reset.css?hash={{$hash}}" integrity="{{$hash}}">
> {{- end}}
> ```
>
</details>

<details><summary><strong>📬 Live updates with Server Sent Events (SSE)</strong></summary>

> Define a template with a name that starts with SSE, like `SSE /url/path`, and
> SSE requests will be handled by invoking the template. Individual messages can
> be sent by using `.Flush`, and the template can be paused to wait on messages
> sent over Go channels or can block on server shutdown.
</details>

<details><summary><strong>🐜 Small footprint</strong></summary>

> Compiles to a ~30MB binary. Easily add your own custom functions and choice of
> database driver on top.
</details>

<details open><summary><strong>🏃‍♂️‍➡️ Single binary deployments</strong></summary>

> Deploy next to your templates and static files or [embed](https://pkg.go.dev/embed)
> them for single binary deployments.
>
> ```go
> //go:embed all:templates
> var Files embed.FS
> ```
>
</details>

## 📦 How to run

### 0. 📦 As a Docker container

...

### 1. 📦 As a Caddy plugin

The `xtemplate/caddy` plugin offers all `xtemplate` features integrated into
[Caddy](https://caddyserver.com), a fast and extensible multi-platform
HTTP/1-2-3 web server with automatic HTTPS.

Download Caddy with `xtemplate/caddy` middleware plugin built-in:

<https://caddyserver.com/download?package=github.com%2Finfogulch%2Fxtemplate&package=github.com%2Fncruces%2Fgo-sqlite3>

This is the simplest Caddyfile that uses the `xtemplate/caddy` plugin:

```Caddyfile
routes {
  xtemplate
}
```

Alternatively, build with the `xcaddy` CLI tool.

### 2. 📦 As the default CLI application

Download from the [Releases page](https://github.com/infogulch/xtemplate/releases) or build the binary in [`./cmd`](./cmd/).

Custom builds can include your chosen db drivers, make Go functions available to
the template definitions, and even embed templates for true single binary
deployments. The [`./cmd` package](./cmd/) is the reference CLI application,
consider starting your customization there.

<details><summary><strong>🎏 CLI flags and examples: (click to show)</strong></summary>

```shell
$ ./xtemplate -h
v0.8.3
Usage: xtemplate [--template-dir TEMPLATE-DIR] [--template-ext TEMPLATE-EXT] [--minify] [--ldelim LDELIM] [--rdelim RDELIM] [--watch WATCH] [--watchtemplates] [--listen LISTEN] [--loglevel LOGLEVEL] [--config CONFIG] [--config-file CONFIG-FILE]

Options:
  --template-dir TEMPLATE-DIR, -t TEMPLATE-DIR [default: templates]
  --template-ext TEMPLATE-EXT [default: .html]
  --minify, -m [default: true]
  --ldelim LDELIM [default: {{]
  --rdelim RDELIM [default: }}]
  --watch WATCH
  --watchtemplates [default: true]
  --listen LISTEN, -l LISTEN [default: 0.0.0.0:8080]
  --loglevel LOGLEVEL [default: -2]
  --config CONFIG, -c CONFIG
  --config-file CONFIG-FILE, -f CONFIG-FILE
  --help, -h             display this help and exit
  --version              display version and exit

Examples:
    Listen on port 80:
    $ ./xtemplate --listen :80

    Specify a context directory and reload when it changes:
    $ ./xtemplate --template-dir public --watch-templates

    Parse template files matching a custom extension and minify them:
    $ ./xtemplate --template-ext ".go.html" --minify
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
  also support path parameters as defined by [http.ServeMux][servemux].
- Template files can be invoked from within other templates using either their
  full path relative to the template root or by using its defined template name.
- Templates are executed with a uniform context object, which provides access to
  request data, database connections, and other useful dynamic functionality.
- Templates can also call functions set at startup.

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

- Access instance data with the `.X` field. See [DotX]
- Access request details with the `.Req` field. See [DotReq]
- Control the HTTP response in buffered template handlers with the `.Resp`
  field. See [DotResp]
- Control flushing behavior for flushing template handlers (i.e. SSE) with the
  `.Flush` field. See [DotFlush]

[DotX]: https://pkg.go.dev/github.com/infogulch/xtemplate#DotX
[DotReq]: https://pkg.go.dev/github.com/infogulch/xtemplate#DotReq
[DotResp]: https://pkg.go.dev/github.com/infogulch/xtemplate#DotResp
[DotFlush]: https://pkg.go.dev/github.com/infogulch/xtemplate#DotFlush

#### ✏️ Optional dot fields

These optional value providers can be configured with any field name, and can be
configured multiple times with different configurations.

- Read and list files. See [DotFS]
- Query and execute SQL statements. See [DotDB]
- Read template-level key-value map. See [DotKV]

[Dir]: https://pkg.go.dev/github.com/infogulch/xtemplate#Dir
[DotDB]: https://pkg.go.dev/github.com/infogulch/xtemplate#DotDB
[DotKV]: https://pkg.go.dev/github.com/infogulch/xtemplate#DotKV

#### ✏️ Custom dot fields

You can create custom dot fields that expose arbitrary Go functionality to your
templates. See [👩‍⚕️ Writing a custom `DotProvider`](#-writing-a-custom-dotprovider).

### 📐 Functions

These are built-in functions that are available to all invocations and don't
depend on request context or mutate state. There are three sets by default:
functions that come by default in the go template library, functions from the
sprig library, and custom functions added by xtemplate.

You can custom FuncMaps by configuring the `Config.FuncMaps` field.

- 📏 `xtemplate` includes funcs to render markdown, sanitize html, convert
  values to human-readable forms, and to try to call a function to handle an
  error within the template. See the free functions named [`FuncXYZ(...)` in
  xtemplate's Go docs][funcgodoc] for details.
- 📏 Sprig publishes a library of useful template funcs that enable templates to
  manipulate strings, integers, floating point numbers, and dates, as well as
  perform encoding tasks, manipulate lists and dicts, converting types,
  and manipulate file paths See [Sprig Function Documentation][sprig].
- 📏 Go's built in functions add logic and basic printing functionality.
  See: [text/template#Functions][gofuncs].

[funcgodoc]: https://pkg.go.dev/github.com/infogulch/xtemplate#FuncHumanize
[sprig]: https://masterminds.github.io/sprig/
[gofuncs]: https://pkg.go.dev/text/template#hdr-Functions

## 🏆 Users

- [PixyBlue/lazy-lob-web](https://github.com/PixyBlue/lazy-lob-web), a fullstack web lob framework.
- [infogulch/xrss](https://github.com/infogulch/xrss), an rss feed reader built with htmx and inline css.
- [infogulch/todos](https://github.com/infogulch/todos), a demo todomvc application.

## 👷‍♀️ Development

### 🗺️ Repository structure

xtemplate is split into the following packages:

- `github.com/infogulch/xtemplate` is a library that exports the `Instance`
  struct which can load template files and implements `http.Handler` that
  routes requests to templates and serves static files, the `Server` struct
  which can atomically reload an `Instance` on demand, and a number of built-in
  providers.
- `./app` is a library that contains an exported `Main` function which
  configures and starts xtemplate with CLI args and accepts config override
  parameters. This `Main` fucntion can be used as a reference for using the
  `xtemplate` API in advanced use-cases.
- `./cmd` is a binary that simply imports a database driver and runs
  `xtemplate/app.Main()`. The recommended way to begin customizing
  xtemplate is to copy the `./cmd` package to your own repo, then add your
  own database driver, provide custom config overrides, etc.
- `./caddy` is a [Caddy module](https://caddyserver.com/docs/extending-caddy)
  package that uses xtemplate's Go library API to integrate xtemplate into Caddy
  server.

> [!TIP]
>
> To understand how the xtemplate package works, it may be helpful to skim
> through the files in this order: [`config.go`](./config.go),
> [`server.go`](./server.go) [`instance.go`](./instance.go),
> [`build.go`](./build.go), [`handlers.go`](./handlers.go).

### Testing

Tasks are managed with [mise](https://mise.jdx.dev). The task definitions live
as [Nushell](https://www.nushell.sh) scripts under
[`.config/mise/tasks`](./.config/mise/tasks) (sharing helpers from
[`.config/mise/lib.nu`](./.config/mise/lib.nu)) and the tool versions (Go, hurl,
Nushell, xcaddy) are pinned in
[`.config/mise/config.toml`](./.config/mise/config.toml), so local and CI runs
use identical versions. List the available tasks with `mise tasks`. Each task is
a standalone Nushell script, so it can also be run directly (e.g.
`./.config/mise/tasks/gotest`) from anywhere in the repo without going through
`mise run`.

The integration tests run xtemplate configured to use `test/templates` as the
templates dir and `test/context` as the FS dot provider, then run the hurl files
from the `test/tests` directory against the running server. The same suite is
exercised against all three deployment targets:

* `mise run test-cli` builds and tests the standalone CLI binary.
* `mise run test-caddy` builds and tests the Caddy module.
* `mise run test-docker` builds and tests the Docker image.

`mise run ci` runs the whole pipeline: lint (`lint-nu` parse-checks the task
scripts with `nu --ide-check`, `lint-go` runs golangci-lint), `go test`
(`gotest`), the three integration targets, and then the release `dist` and
Docker image builds.

### 👩‍⚕️ Writing a custom `DotProvider`

Implement the `xtemplate.DotConfig` interface on your type:

```go
type DotConfig interface {
    FieldName() string            // the dot field name, e.g. "Shop" for {{.Shop}}
    Init(context.Context) error   // called once at instance load
    Value(Request) (any, error)   // called per request; its return is assigned to the dot field
}
```

Register it by passing `xtemplate.WithProvider(yourConfig)` as an option to
`config.Server(...)`, `config.Instance(...)`, or `app.Main(...)`:

```go
app.Main(xtemplate.WithProvider(&ShopConfig{}))
```

On startup xtemplate creates a struct that includes your value as a field named
by `FieldName()`. For every request your `Value` method is called with request
details and its return value is assigned onto that struct, which is passed to
`html/template` as the dot value `{{.}}`. `Value` must return a stable, non-nil
concrete type: it is called once with a mock request at load time to infer the
field type via reflection.

Optionally implement `xtemplate.CleanupDotProvider` to run per-request cleanup
(e.g. rolling back a transaction), the way the built-in `DotDB` provider does.

See [`examples/dotprovider`](./examples/dotprovider/) for a complete, runnable
example.

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

xtemplate has since been refactored to be usable independently from Caddy, and
is published as a subpackage in this module at [./caddy](./caddy) which uses
the public xtemplate Go API and to integrate xtemplate into Caddy as an
http middleware.

See [CHANGELOG.md](./CHANGELOG.md) for a per-release history of notable changes.

`xtemplate` is licensed under the Apache 2.0 license. See [LICENSE](./LICENSE)

[go-htmx]: https://github.com/infogulch/go-htmx
[caddyhttp-templates]: https://github.com/caddyserver/caddy/tree/master/modules/caddyhttp/templates
