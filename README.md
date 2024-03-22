# xtemplate

`xtemplate` is a html/template-based hypertext preprocessor and rapid
application development web server written in Go. It streamlines construction of
hypermedia-exchange-oriented web sites by efficiently handling basic server
tasks, enabling authors to focus on defining routes and responding to them using
templates and configurable data sources.

## 💡 Why?

After bulding some sites with [htmx](https://htmx.org) and Go, I wished that
everything would just get out of the way of the absolute fundamentals:

- URLs and path patterns
- Access to a backing data source
- Executing a template to return HTML

**The hypothesis of `xtemplate` is that *templates* can be the nexus of these
fundamentals.**

The Go stdlib `html/template` package is a good foundation, but it requires some
acute customization and a decent surrounding implementation to realize a vision
of 'templates as the engine of backend server state'.

## 🎇 Features

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
> allowed to complete *gracefully*.
>
> Add this template definition and one-line script to your page, then
> clients will automatically reload when the server does:
>
> ```html
> {{- define "SSE /reload"}}{{.Block}}data: reload{{printf "\n\n"}}{{end}}
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

<details open><summary><strong>📤 Optimal static file serving</strong></summary>

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

## 👨‍🏫 How it works

xtemplate is designed to treat the files in the templates directory as
relatively static. When a file under the templates directory is modified the
server will quickly reload and can be configured to automatically refresh
browser clients making the development experience very snappy. Templates can
have dynamic access to a directory in the local filesystem if you set set the
"context path" config. It may be helpful to consider the templates and context
directories to be 'application definition' and 'user storage', respectively.

When [`config.Instance()`](instance.go) is called, xtemplate recursively scans
all files in the templates directory and loads them in. Except for hidden files
where its filename starts with `.`, all of these files are added to a Go 1.22
`http.ServeMux`: static files by their full path, and template files by their
path minus extension.

* Template files (files with extension `.html`; configurable) are loaded into a
  custom `html/template.Template` where all defined templates are available to
  invoke from all other templates, and all files are available to include in any
  other template by their full path relative to the template directory. Each
  template file is added as the route handler for that path. If any template
  file defines a named template where the name matches a ServeMux pattern like
  `{{define "GET /path"}}...{{end}}` or `{{define "DELETE
  /items/{id}"}}...{{end}}`, then that pattern is also added to the routes and
  the named template definition will be used as the handler.
* All other files are considered "static files" and are served directly from
  disk. The file is hashed, which is used to optimize caching behavior by
  handling cache management headers like `Etag` and `If-None-Match`. If the file
  has a compressed version (e.g. `file.txt` and `file.txt.gz`), after validating
  that both have equivalent contents, the server will negotiate with clients to
  select the best encoding when requesting the path of the original file in
  order to optimize bandwidth and cpu.

In response to a request, the [bufferedTemplateHandler](handlers.go) executes
the associated template, writing the output into a buffer; if the template
execution succeeds then the buffer is written out as the response to the
request. The template is executed with dynamic data in the dot context: request
details at [`.Req`](#request-data-and-response-control), database access at
[`.Tx` and `.Query`](#database-functions), and filesystem access like
[`.ReadFile`](#file-operations), depending on config (see [context](#-context)).
Arbitrary plain go functions can be added to to the templating engine with the
`FuncMaps` config, in addition to many useful [default funcs](#-functions) added
by xtemplate like functions to escape html (thanks
[BlueMonday](https://github.com/microcosm-cc/bluemonday/)), render markdown
(thanks [Goldmark](https://github.com/yuin/goldmark)), or manipulate lists
(thanks [Sprig](https://masterminds.github.io/sprig/)).

## 👨‍🏭 How to use

### 📦 Deployment modes

XTemplate can be used in various modes:

#### 1. 📦 As the default CLI application

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

#### 2. 📦 As a Go library

xtemplate's public Go API starts with a [`xtemplate.Config`](./config.go),
from which you can get either an [`xtemplate.Instance`](./instance.go) interface
or a [`xtemplate.Server`](./server.go) interface, with the methods
`config.Instance()` and `config.Server()`, respectively.

An `xtemplate.Instance` is an immutable `http.Handler` that can handle requests,
and exposes some metadata about the files loaded as well as the ServeMux
patterns and associated handlers for individual routes. An `xtemplate.Server`
also handles http requests by forwarding requests to an internal Instance, but
the `Server` can be reloaded by calling `server.Reload()`, which creates a new
Instance and atomically switches the handler to direct new requests to the new
Instance.

Use an Instance if you have no interest in reloading, or if you want to use
xtemplate handlers in your own mux. Use a Server if you want an easy way to
smoothly reload and replace the xtemplate Instance behind a http.Handler at
runtime.

#### 3. 📦 As a Caddy plugin

The `xtemplate-caddy` plugin offers all `xtemplate` features integrated into
[Caddy](https://caddyserver.com), a fast and extensible multi-platform
HTTP/1-2-3 web server with automatic HTTPS.

Download Caddy with `xtemplate-caddy` middleware plugin built-in:

https://caddyserver.com/download?package=github.com%2Finfogulch%2Fxtemplate-caddy

This is the simplest Caddyfile that uses the `xtemplate-caddy` plugin:
`caddy-xtemplate` is a [Caddy](https://caddyserver.com) module that extends
Go's [`html/template` library](https://pkg.go.dev/html/template) to be capable
enough to host an entire server-side application in it. Designed with the
[htmx.org](https://htmx.org/) js library in mind, which makes server side
rendered sites feel as interactive as a Single Page Apps.

### 🧰 Template semantics

Templates are loaded using Go's [`html/template`](https://pkg.go.dev/html/template)
module, which is extended with additional functions and a specific context.

Unlike the html/template default, all template files are recursively loaded into
the same persistent templates instance. Requests are served by invoking the
matching template name that matches the request route and path. Templates can
also be invoked from another template by full path name rooted at template root,
or by explicitly defined templates by any name.

Route handling templates are invoked with a uniform root context `{{$}}` which
includes request data, local file access (if configured), and db access (if
configured). (See the next section for details.) When the template finishes the
result is sent to the client.
Write `.html` files in the root directory specified in your Caddy config.

`xtemplate-caddy` is built on top of the [library API](#-as-a-go-library).

### 📝 Context

The dot context `{{.}}` set on each template invocation provides access to
request-specific data and response control methods, as well as any custom

#### Access request details with `.Req`

`.Req` contains the current HTTP request struct,
[http.Request](https://pkg.go.dev/net/http#Request), which can be used to read
the detailed contents of the request. Some notable methods are mentioned here:

- `.Req.Method` The HTTP method of the request
- `.Req.URL` The URI being requested
  - `.Req.URL.Query` Query values as `url.Values`
  - `.Req.URL.Host` host or host:port
  - See also `Path`, `User`, `Scheme`, `Hostname`, `Port`
- `.Req.Header` - Request header fields
  - `.Req.Header "HeaderName"` Get the list of all values with the header name
  - `.Req.Header.Get "HeaderName"` Gets the first value with the header name
- `.Req.Cookie "cookiename"` Gets the value of a cookie by name.
- `.Req.PathValue` is a list of path parameters extracted from the url based on the current route
- `.Req.Host` - the Host header or :authority of the request
- `.Req.RemoteIP` is the client's IP address
- `.Req.ParseForm` parses the url query and request body for form content and
  updates `Form` and `PostForm` fields. Must be called before `.Form` or `.PostForm`.
- `.Req.Form` contains the parsed form data, including both the URL field's
  query parameters and the PATCH, POST, or PUT form data
- See also `RemoteAddr`, `PostForm`, `FormValue`

#### Control the http response with `.Resp`

`.Resp` contains methods to control the buffered template response:

- `.Resp.SetStatus 201` Set the status code of the current response if no error
  occurs during template rendering.
- `.Resp.ReturnStatus 200` Sets the status code and aborts template execution.
- `.Resp.AddHeader "Header-Name" $val` Adds a header field to the HTTP response
- `.Resp.SetHeader "Header-Name" $val` Sets a header field on the HTTP response,
  replacing any existing values
- `.Resp.DelHeader "Header-Name"` Deletes a header field on the HTTP response
- `.Resp.ServeContent $path $modtime $content`, like `.ServeFile`, discards any
  template content rendered so far, and responds to the request with the raw
  string `$content`. Intended to serve rendered documents by responding with 304
  Not Modified by coordinating with the client on `$modtime`.

#### File operations

File operations are rooted at the directory specified by the `context_root`
config option. If a context root is not configured these functions produce an
error.

- `.ReadFile $file` reads and returns the contents of a file, as a string.
- `.ListFiles $file` returns a list of the files in the given directory.
- `.FileExists $file` returns true if filename can be opened successfully.
- `.StatFile $file` returns Stat of a filename.
- `.ServeFile $file` discards any template content rendered so far and responds to the request with the contents of the file at `$file`

#### Database functions

All funcs accept a query string and any number of parameters. Prefer using parameters over building the query string dynamically.

- `.Exec` executes a statment
- `.QueryRows` executes a query and returns all rows in a big `[]map[string]any`.
- `.QueryRow` executes a query, which must return one row, and returns the `map[string]any`.
- `.QueryVal` executes a query, which must return one row and one column, and returns the value of the column.

#### Other

- `.Template` evaluate the template name with the given context and return the result as a string.
- `.Funcs` returns a list of all the custom FuncMap funcs that are available to call. Useful in combination with the `try` func.
- `.Config` is a map of config strings set in the Caddyfile. See [Config](#-config).


### 📐 Functions

These are built-in functions that are available to all invocations and don't
depend on request context or mutate state. There are three sets by default:
functions that come by default in the go template library, functions from the
sprig library, and custom functions added by xtemplate.

You can custom FuncMaps by setting `config.FuncMaps = myFuncMap` or calling
`xtemplate.Main(xtemplate.WithFuncMaps(myFuncMap))`.

<details><summary><strong>Go stdlib template functions</strong></summary>

See [text/template#Functions](https://pkg.go.dev/text/template#hdr-Functions).

- `and`
  Returns the boolean AND of its arguments by returning the
  first empty argument or the last argument. That is,
  "and x y" behaves as "if x then y else x."
  Evaluation proceeds through the arguments left to right
  and returns when the result is determined.
- `call`
  Returns the result of calling the first argument, which
  must be a function, with the remaining arguments as parameters.
  Thus "call .X.Y 1 2" is, in Go notation, dot.X.Y(1, 2) where
  Y is a func-valued field, map entry, or the like.
  The first argument must be the result of an evaluation
  that yields a value of function type (as distinct from
  a predefined function such as print). The function must
  return either one or two result values, the second of which
  is of type error. If the arguments don't match the function
  or the returned error value is non-nil, execution stops.
- `html`
  Returns the escaped HTML equivalent of the textual
  representation of its arguments. This function is unavailable
  in html/template, with a few exceptions.
- `index`
  Returns the result of indexing its first argument by the
  following arguments. Thus "index x 1 2 3" is, in Go syntax,
  x[1][2][3]. Each indexed item must be a map, slice, or array.
- `slice`
  slice returns the result of slicing its first argument by the
  remaining arguments. Thus "slice x 1 2" is, in Go syntax, x[1:2],
  while "slice x" is x[:], "slice x 1" is x[1:], and "slice x 1 2 3"
  is x[1:2:3]. The first argument must be a string, slice, or array.
- `js`
  Returns the escaped JavaScript equivalent of the textual
  representation of its arguments.
- `len`
  Returns the integer length of its argument.
- `not`
  Returns the boolean negation of its single argument.
- `or`
  Returns the boolean OR of its arguments by returning the
  first non-empty argument or the last argument, that is,
  "or x y" behaves as "if x then x else y".
  Evaluation proceeds through the arguments left to right
  and returns when the result is determined.
- `print`
  An alias for fmt.Sprint
- `printf`
  An alias for fmt.Sprintf
- `println`
  An alias for fmt.Sprintln
- `urlquery`
  Returns the escaped value of the textual representation of
  its arguments in a form suitable for embedding in a URL query.
  This function is unavailable in html/template, with a few
  exceptions.

</details>

<details><summary><strong>Sprig library template functions</strong></summary>

See the Sprig documentation for details: [Sprig Function Documentation](https://masterminds.github.io/sprig/).

- [String Functions](https://masterminds.github.io/sprig/strings.html):
  - `trim`, `trimAll`, `trimSuffix`, `trimPrefix`, `repeat`, `substr`, `replace`, `shuffle`, `nospace`, `trunc`, `abbrev`, `abbrevboth`, `wrap`, `wrapWith`, `quote`, `squote`, `cat`, `indent`, `nindent`
  - `upper`, `lower`, `title`, `untitle`, `camelcase`, `kebabcase`, `swapcase`, `snakecase`, `initials`, `plural`
  - `contains`, `hasPrefix`, `hasSuffix`
  - `randAlphaNum`, `randAlpha`, `randNumeric`, `randAscii`
  - `regexMatch`, `mustRegexMatch`, `regexFindAll`, `mustRegexFindAll`, `regexFind`, `mustRegexFind`, `regexReplaceAll`, `mustRegexReplaceAll`, `regexReplaceAllLiteral`, `mustRegexReplaceAllLiteral`, `regexSplit`, `mustRegexSplit`, `regexQuoteMeta`
  * [String List Functions](https://masterminds.github.io/sprig/strings.html): `splitList`, `sortAlpha`, etc.
- [Integer Math Functions](https://masterminds.github.io/sprig/math.html): `add`, `max`, `mul`, etc.
  - [Integer Slice Functions](https://masterminds.github.io/sprig/integer_slice.html): `until`, `untilStep`
- [Float Math Functions](https://masterminds.github.io/sprig/mathf.html): `addf`, `maxf`, `mulf`, etc.
- [Date Functions](https://masterminds.github.io/sprig/date.html): `now`, `date`, etc.
- [Defaults Functions](https://masterminds.github.io/sprig/defaults.html): `default`, `empty`, `coalesce`, `fromJson`, `toJson`, `toPrettyJson`, `toRawJson`, `ternary`
- [Encoding Functions](https://masterminds.github.io/sprig/encoding.html): `b64enc`, `b64dec`, etc.
- [Lists and List Functions](https://masterminds.github.io/sprig/lists.html): `list`, `first`, `uniq`, etc.
- [Dictionaries and Dict Functions](https://masterminds.github.io/sprig/dicts.html): `get`, `set`, `dict`, `hasKey`, `pluck`, `dig`, `deepCopy`, etc.
- [Type Conversion Functions](https://masterminds.github.io/sprig/conversion.html): `atoi`, `int64`, `toString`, etc.
- [Path and Filepath Functions](https://masterminds.github.io/sprig/paths.html): `base`, `dir`, `ext`, `clean`, `isAbs`, `osBase`, `osDir`, `osExt`, `osClean`, `osIsAbs`
- [Flow Control Functions](https://masterminds.github.io/sprig/flow_control.html): `fail`
- Advanced Functions
  - [UUID Functions](https://masterminds.github.io/sprig/uuid.html): `uuidv4`
  - [OS Functions](https://masterminds.github.io/sprig/os.html): `env`, `expandenv`
  - [Version Comparison Functions](https://masterminds.github.io/sprig/semver.html): `semver`, `semverCompare`
  - [Reflection](https://masterminds.github.io/sprig/reflection.html): `typeOf`, `kindIs`, `typeIsLike`, etc.
  - [Cryptographic and Security Functions](https://masterminds.github.io/sprig/crypto.html): `derivePassword`, `sha256sum`, `genPrivateKey`, etc.
  - [Network](https://masterminds.github.io/sprig/network.html): `getHostByName`

</details>

<details><summary><strong>xtemplate functions</strong></summary>

See [funcs.go](/funcs.go) for details.

- `markdown` Renders the given Markdown text as HTML and returns it. This uses the [Goldmark](https://github.com/yuin/goldmark) library, which is CommonMark compliant. It also has these extensions enabled: Github Flavored Markdown, Footnote, and syntax highlighting provided by [Chroma](https://github.com/alecthomas/chroma).
- `splitFrontMatter` Splits front matter out from the body. Front matter is metadata that appears at the very beginning of a file or string. Front matter can be in YAML, TOML, or JSON formats.
  - `.Meta` to access the metadata fields, for example: `{{$parsed.Meta.title}}`
  - `.Body` to access the body after the front matter, for example: `{{markdown $parsed.Body}}`
- `sanitizeHtml` Uses [bluemonday](https://github.com/microcosm-cc/bluemonday/) to sanitize strings with html content. `{{sanitizeHtml "strict" "Shows <b>only</b> text content"}}`
  - First parameter is the name of the chosen sanitization policy. `"strict"` = [`StrictPolicy()`](https://github.com/microcosm-cc/bluemonday/blob/main/policies.go#L38C6-L38C20), `"ugc"` = [`UGCPolicy()`](https://github.com/microcosm-cc/bluemonday/blob/main/policies.go#L54C6-L54C17) for 'user generated content', `"externalugc"` = `UGCPolicy()` + disallow relative urls + add target=_blank to urls.
  - Second parameter is the content to sanitize.
  - Returns the string as a `template.HTML` type which can be output directly into the document without `trustHtml`.
- `humanize` Transforms size and time inputs to a human readable format using the [go-humanize](https://github.com/dustin/go-humanize) library. Call with two parameters, the format type and the value to format. Format types are:
  - **size** which turns an integer amount of bytes into a string like `2.3 MB`, for example: `{{humanize "size" "2048000"}}`
  - **time** which turns a time string into a relative time string like `2 weeks ago`, for example: `{{humanize "time" "Fri, 05 May 2022 15:04:05 +0200"}}`
- `ksuid` returns a 'K-Sortable Globally Unique ID' using [segmentio/ksuid](https://github.com/segmentio/ksuid)
- `idx` gets an item from a list, similar to the built-in `index`, but with reversed args: index first, then array. This is useful to use index in a pipeline, for example: `{{generate-list | idx 5}}`
- `try` takes a function that returns an error in the first argument and calls it with the values from the remaining arguments, and returns the result including any error as struct fields. This enables template authors to handle funcs that return errors within the template definition. Example: `{{ $result := try .QueryVal "SELECT 'oops' WHERE 1=0" }}{{if $result.OK}}{{$result.Value}}{{else}}QueryVal requires exactly one row. Error: {{$result.Error}}{{end}}`

</details>

## 🏆 Users

* [infogulch/xrss](https://github.com/infogulch/xrss), an rss feed reader built with htmx and inline css.
* [infogulch/todos](https://github.com/infogulch/todos), a demo todomvc application.

## 👷‍♀️ Development

xtemplate is split into the following packages:

* `github.com/infogulch/xtemplate`, a library that loads template files and
  implements an `http.Handler` that routes requests to templates and serves
  static files.
* `github.com/infogulch/xtemplate/cmd`, a simple binary that configures
  `xtemplate` with CLI args and serves http requests with it.
* [`github.com/infogulch/xtemplate-caddy`](https://github.com/infogulch/xtemplate-caddy),
  uses xtemplate's Go library API to integrate xtemplate into Caddy server as a
  Caddy module.

xtemplate is tested by running [`./test/exec.sh`](./test/exec.sh) which loads
using the `test/templates` and `test/context` directories, and runs hurl files
from the `test/tests` directory.

> [!TIP]
>
> To understand how the xtemplate package works, it may be helpful to skim
> through the files in this order: [`main.go`](./main.go),
> [`config.go`](./config.go), [`server.go`](./server.go)
> [`instance.go`](./instance.go), [`build.go`](./build.go),
> [`handlers.go`](./handlers.go).

## ✅ Project history and license
# Development

## 🏆 Known users

* [infogulch/xrss](https://github.com/infogulch/xrss), an rss feed reader built with htmx and inline css.
* [infogulch/todos](https://github.com/infogulch/todos), a demo todomvc application.

## 👷‍♀️ Development

xtemplate is split into the following packages:

* `github.com/infogulch/xtemplate`, a library that loads template files and
  implements an `http.Handler` that routes requests to templates and serves
  static files.
* `github.com/infogulch/xtemplate/cmd`, a simple binary that configures
  `xtemplate` with CLI args and serves http requests with it.
* [`github.com/infogulch/xtemplate-caddy`](https://github.com/infogulch/xtemplate-caddy),
  uses xtemplate's Go library API to integrate xtemplate into Caddy server as a
  Caddy module.

xtemplate is tested by running [`./test/exec.sh`](./test/exec.sh) which loads
using the `test/templates` and `test/context` directories, and runs hurl files
from the `test/tests` directory.

> [!TIP]
>
> To understand how the xtemplate package works, it may be helpful to skim
> through the files in this order: [`main.go`](./main.go),
> [`config.go`](./config.go), [`server.go`](./server.go)
> [`instance.go`](./instance.go), [`build.go`](./build.go),
> [`handlers.go`](./handlers.go).

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
