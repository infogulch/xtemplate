# XTemplate

`xtemplate` is an html/http server that simplifies server side web development,
building on Go's `html/template` library to streamline construction of a
hypermedia-exchange-oriented web server using just template definitions.
Entirely eschew defining and naming handlers, param structs, query row structs,
and template context structs. Completely circumvent manual management of route
tables, discovering and loading template files, and serving static files. Do all
of this with a development loop measured in milliseconds and response times
measured in microseconds.

### Table of contents

- 🎇 [Features](#features)
- 📦 [How to use](#how-to-use)
- 🏆 [Showcase](#showcase)
- 📐 [Template syntax](#template-syntax)
  - [Context values](#context-values)
  - [Functions](#functions)
    - [Stdlib Functions](#go-stdlib-template-functions)
    - [Sprig Functions](#sprig-library-template-functions)
    - [xtemplate Functions](#xtemplate-functions)
- 🛠️ [Development](#development)
- ✅ [License](#project-history-and-license)

> This project is still in development.

## Features

*Click a feature to expand and show details:*

<details open><summary><strong>📜 Execute database queries directly within a template</strong></summary>

> ```html
> <ul>
>   {{range .Query `SELECT id,name FROM contacts`}}
>   <li><a href="/contact/{{.id}}">{{.name}}</a></li>
>   {{end}}
> </ul>
> ```
>
> The html/template library automatically sanitizes inputs, so you can
> rest easy from basic XSS attacks. But if you generate some html that you do
> trust, it's easy to inject if you intend to.
</details>

<details><summary><strong>🗺️ Default file-based routing</strong></summary>

> `GET` requests for a path with a matching template file will invoke the
> template file at that path.
>
> Except files starting with `_` are not routed. This allows defining templates
> that only other templates can invoke. (See the next feature)
>
> ```
> .
> ├── index.html          GET /
> ├── todos.html          GET /todos
> ├── admin
> │   └── settings.html   GET /admin/settings
> └── shared
>     └── _head.html      (not routed because it starts with '_')
> ```
</details>

<details><summary><strong>⤵️ Define and invoke templates anywhere</strong></summary>

> All html files under the template root directory are available to invoke by
> their full path relative to the template root dir starting with `/`.
>
>
>
> ```html
> <html>
>   <title>Home</title>
>   <!-- import the contents of another file -->
>   {{template "/shared/_head.html" .}}
>
>   <body>
>     <!-- invoke a custom template defined anywhere -->
>     {{template "navbar" .}}
>     ...
>   </body>
> </html>
> ```
</details>

<details><summary><strong>🚦 Custom routes can handle any method</strong></summary>

> Create custom route handlers for any http method and parametrized path by
> defining a template whose name matches the pattern `<method> <path>`. Uses
> [httprouter](https://github.com/julienschmidt/httprouter) syntax for path
> parameters and wildcards, which are made available in the template as values
> in the `.Param` key while serving a request.
>
> ```html
> <!-- match on path parameters -->
> {{define "GET /contact/:id"}}
> {{$contact := .QueryRow `SELECT name,phone FROM contacts WHERE id=?` (.Params.ByName "id")}}
> <div>
>   <span>Name: {{$contact.name}}</span>
>   <span>Phone: {{$contact.phone}}</span>
> </div>
> {{end}}
>
> <!-- match on any http method -->
> {{define "DELETE /contact/:id"}}
> {{$_ := .Exec `DELETE from contacts WHERE id=?` (.Params.ByName "id")}}
> {{.RespStatus 204}}
> {{end}}
> ```
</details>

<details open><summary><strong>➰ Automatic reload</strong></summary>

> By default templates are reloaded and validated automatically as soon as they
> are modified, no need to restart the server. If an error occurs during load it
> continues to serve the old version and outputs the error in the log.
>
> With this two line template you can configure your page to automatically reload
> when the server reloads. (Great for development, maybe not great for prod.)
>
> ```html
> <script>new EventSource("/reload").onmessage = () => location.reload()</script>
> {{- define "SSE /reload"}}{{.Block}}data: reload{{printf "\n\n"}}{{end}}
> ```
</details>

<details open><summary><strong>🗄️ Ideal static file serving</strong></summary>

> All non-.html files in the templates directory are considered static files and
> are served directly from disk with valid handling and 304 responses based on
> ETag/If-Match/If-None-Match and Modified/If-Modified-Since headers.
>
> If a static file also has .gz, .br, .zip, or .zst copies they are decoded and
> hashed for consistency on startup and are automatically served directly from
> disk to the client with a negotiated `Accept-Encoding` and appropriate
> `Content-Encoding`.
>
> Templates can efficiently access the static file's precalculated content hash
> to build a `<script>` or `<link>` integrity attribute, instructing clients to
> check the integrity of the content if they are served through a CDN. See:
> [Subresource Integrity](https://developer.mozilla.org/en-US/docs/Web/Security/Subresource_Integrity)
>
> Add the content hash as a query parameter and responses will automatically add
> a 1 year long `Cache-Control` header. This is causes clients to cache as long
> as possible, and if the file changes its hash will change so its query
> parameter will change so the client will immediately request a new version,
> seamlessly sidestepping stale cache issues.
>
> ```html
> {{- with $hash := .StaticFileHash `/reset.css`}}
> <link rel="stylesheet" href="/reset.css?hash={{$hash}}" integrity="{{$hash}}">
> {{- end}}
> ```

</details>

<details><summary><strong>📩 Live updates with Server Sent Events (SSE)</strong></summary>

> TODO
</details>

<details><summary><strong>🐜 Small footprint, easy to deploy</strong></summary>

> Compiles to a small ~30MB binary. Easily add your own custom functions and
> choice of database driver on top. Deploy next to your templates and static
> files or [embed](https://pkg.go.dev/embed) them into the binary for single
> binary deployments.
</details>

# How to use

> TODO: This section is a work in progress, but here is a basic overview:

* XTemplate is flexible and can be used in several different modes:
  * As a library: call `New()`, use functional options to configure it, then call `.Build()` to get an `http.Handler`; do whatever you like with it. See `config.go`.
  * As a standalone binary, run `go build ./cmd`
  * As a custom binary, see `main.go:Main()` and `./cmd/main.go` as an example. Do this if you want to use a custom db driver, custom functions, or embed your templates for single binary deployment.
  * As an [http middlware plugin](https://caddyserver.com/download?package=github.com%2Finfogulch%2Fxtemplate%2Fcaddy) for [Caddy Server](https://caddyserver.com/)
* Configuration
  * Template files and static files are loaded from the template root directory, configured by `--template-root` (default "templates")
  * If you want the templates to have access to the local file system, configure it with a `--context-root` directory path (default disabled)
  * If you want database access, configure it with `--db-driver` and `--db-connstr`
  * See other configuration options in `config.go` and flags in `main.go`
* Usage
  * Unlike the html/template default, all template files are recursively loaded into the same templates instance. They can be invoked from another template by full path name rooted at template root.
  * .html template files are automatically invoked upon request to their path minus extension
  * Templates are invoked with a consistent root context which includes request data, local file access (if configured), and db access (if configured). (See the next section for details.) When the template finishes the result is sent to the client.
  * Other files are served as static files
  * Create extra handlers besides file-based routes by defining template names that match an http method and path with path wildcards or methods other than GET
  * Define SSE handler with SSE prefix
* For detailed documentation about available functions and context, see the next section: [Template sytax](#template-syntax).

# Template syntax

Templates are loaded using Go's [`html/template`](https://pkg.go.dev/html/template)
module, which is extended with additional functions and a specific context.

### Context values

The dot context `{{.}}` set on the main template handler provides
request-specific data and stateful actions. See [tplcontext.go](tpl.context.go)
for details.

- Request and response related fields and fields
  - `.Req` is the current HTTP request struct, [http.Request](https://pkg.go.dev/net/http#Request), which has various fields, including:
    - `.Method` - the method
    - `.URL` - the URL, which in turn has component fields (Scheme, Host,
      Path, etc.)
    - `.Header` - the header fields
    - `.Host` - the Host or :authority header of the request
  - `.Params` is a list of path parameters extracted from the url based on the
    current route, see [custom routes](#file-based-routing--custom-routes). `{{.Params.ByName "id"}}`
  - `.RemoteIP` is the client's IP address. `{{.RemoteIP}}`
  - `.Host` is the hostname portion (no port) of the Host header of the HTTP request.
  - `.Cookie` Gets the value of a cookie by name. `{{.Cookie "cookiename"}}`
  - `.Placeholder` gets a caddy "placeholder variable". The braces (`{}`)
    have to be omitted.
  - `.SetStatus` Set the status code of the current response. `{{.RespStatus 201}}`
  - `.AddHeader` Adds a header field to the HTTP response. `{{.RespHeader.Add "Field-Name" "val"}}`
  - `.SetHeader` Sets a header field on the HTTP response, replacing any existing value.
  - `.DelHeader` Deletes a header field on the HTTP response.
- File related funcs. File operations are rooted at the directory specified by the `context_root` config option.
  - `.ReadFile $file` reads and returns the contents of a file, as-is. Note that the contents are NOT escaped, so you should only read trusted files.
  - `.ListFiles` returns a list of the files in the given directory.
  - `.FileExists` returns true if filename can be opened successfully.
  - `.StatFile` returns Stat of a filename.
  - `.ServeFile` discards any template content rendered so far and responds to the request with the contents of the file at
- Database related funcs. All funcs accept a query string and any number of parameters. Prefer using parameters over building the query string dynamically.
  - `.Exec` executes a statment
  - `.QueryRows` executes a query and returns all rows in a big `[]map[string]any`.
  - `.QueryRow` executes a query, which must return one row, and returns the `map[string]any`.
  - `.QueryVal` executes a query, which must return one row and one column, and returns the value of the column.
- Other
  - `.Template` evaluate the template name with the given context and return the result as a string.
  - `.Funcs` returns a list of all the custom FuncMap funcs that are available to call. Useful in combination with the `try` func.
  - `.Config` is a map of config strings set in the Caddyfile. See [Config](#config).

### Functions

These are built-in functions that are available to all invocations and don't
depend on request context or mutate state. There are three sets by default:
functions that come by default in the go template library, functions from the
sprig func library, and extra functions added by xtemplate.

You can also add your own custom FuncMap by calling the
`config.WithFuncMaps(myfuncmap)` while constructing `XTemplate`.

<details><summary><h4>Go stdlib template functions</h4></summary>

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

<details><summary><h4>Sprig library template functions</h4></summary>

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

<details><summary><h4>XTemplate functions</h4></summary>

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

# Showcase

* [infogulch/xrss](https://github.com/infogulch/xrss), an rss feed reader built with htmx and inline css.
* [infogulch/todos](https://github.com/infogulch/todos), a demo todomvc application.

## Development

XTemplate is three Go packages in this repository:

* `github.com/infogulch/xtemplate`, a library that loads template files and implements `http.Handler`, routing requests to templates and serving static files. Use it in your own Go application by depending on it as a library and using the [`New` func](config.go) and functional options.
* `github.com/infogulch/xtemplate/cmd`, a simple binary that configures `XTemplate` with CLI args and serves http requests with it. Build it yourself or download the binary from github releases.
* `github.com/infogulch/xtemplate/caddy`, a caddy module that integrates xtemplate into the caddy web server.

You can register custom functions for use in the default cli by registering using the `./register` module. This module scheme prevents polluting your custom function module with dozens of xtemplate-specific dependencies.

XTemplate is tested by running `./integration/run.sh` which loads the test templates and context directory, and runs hurl tests from the tests directory.

## Project history and license

The idea for this project started as [infogulch/go-htmx][go-htmx] (now
archived), which included the first implementations of template-name-based
routing, exposing sql db functions to templates, and a persistent templates
instance shared across requests and reloaded when template files changed.

go-htmx was refactored and rebased on top of the [templates module from the
Caddy server][caddyhttp-templates] to create `caddy-xtemplate` to add some extra
features including reading files directly and built-in funcs for markdown
conversion, and to get a jump start on supporting the broad array of web server
features without having to implement them from scratch.

`xtemplate` is licensed under the Apache 2.0 license. See [LICENSE](./LICENSE)

[go-htmx]: https://github.com/infogulch/go-htmx
[caddyhttp-templates]: https://github.com/caddyserver/caddy/tree/master/modules/caddyhttp/templates
