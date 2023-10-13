# XTemplate

`xtemplate` is a hypertext preprocessor that extends Go's
[`html/template` library][html-template] to be capable enough to host an entire
server-side web application using just template definitions. Designed with the
[htmx.org][htmx] js library in mind, which makes server side rendered sites feel
as interactive as a Single Page Apps.

[html-template]: https://pkg.go.dev/html/template
[htmx]: https://htmx.org/

> ⚠️ This project is still in development. ⚠️

> ---
>
> ### Table of contents
>
> - ✨ [Features](#features)
>   - [Query the database directly within template definitions](#query-the-database-directly-within-template-definitions)
>   - [Define templates and import content from other files](#define-templates-and-import-content-from-other-files)
>   - [File-based routing](#file-based-routing)
>   - [Custom routes](#custom-routes)
>   - [Automatic reload](#automatic-reload)
> - 🏆 [Showcase](#showcase)
> - 📦 [How to use](#how-to-use)
> - 📐 [Template syntax](#template-syntax)
>   - [Context values](#context-values)
>   - [Functions](#functions)
>     - [Stdlib Functions](#go-stdlib-template-functions)
>     - [Sprig Functions](#sprig-library-template-functions)
>     - [xtemplate Functions](#xtemplate-functions)
> - 🛠️ [Development](#development)
> - ✅ [License](#project-history-and-license)
>
> ---

## Features

### Query the database directly within template definitions

```html
<ul>
  {{range .Query `SELECT id,name FROM contacts`}}
  <li><a href="/contact/{{.id}}">{{.name}}</a></li>
  {{end}}
</ul>
```

> Note: The html/template library automatically sanitizes inputs, so you can
> rest easy from basic XSS attacks. Note: if you generate some html that you do
> trust, it's easy to inject if you intend to.

### Define templates and import content from other files

```html
<html>
  <title>Home</title>
  <!-- import the contents of another file -->
  {{template "/shared/_head.html" .}}

  <body>
    <!-- invoke a custom template defined anywhere -->
    {{template "navbar" .}}
    ...
  </body>
</html>
```

### File-based routing

`GET` requests for a path with a matching template file will invoke the template
file at that path, with the exception that files starting with `_` are not
routed. This allows defining templates that only other templates can invoke.

```
.
├── index.html          GET /
├── todos.html          GET /todos
├── admin
│   └── settings.html   GET /admin/settings
└── shared
    └── _head.html      (not routed)
```

### Custom routes

Create custom route handlers for any http method and parametrized path by
defining a template whose name matches the pattern `<method> <path>`. Uses
[httprouter](https://github.com/julienschmidt/httprouter) syntax for path
parameters and wildcards, which are made available in the template as values in
the `.Param` key while serving a request.

```html
<!-- match on path parameters -->
{{define "GET /contact/:id"}}
{{$contact := .QueryRow `SELECT name,phone FROM contacts WHERE id=?` (.Params.ByName "id")}}
<div>
  <span>Name: {{$contact.name}}</span>
  <span>Phone: {{$contact.phone}}</span>
</div>
{{end}}

<!-- match on any http method -->
{{define "DELETE /contact/:id"}}
{{$_ := .Exec `DELETE from contacts WHERE id=?` (.Params.ByName "id")}}
{{.RespStatus 204}}
{{end}}
```

### Automatic reload

Templates are reloaded and validated automatically as soon as they are modified,
no need to restart the server. If there's a syntax error it continues to serve
the old version and prints the loading error out in the logs.

> Ctrl+S > Alt+Tab > F5

# Showcase

* [infogulch/xrss](https://github.com/infogulch/xrss), an rss feed reader built with htmx and inline css.
* [infogulch/todos](https://github.com/infogulch/todos), a demo todomvc application.

# How to use

XTemplate is three Go packages in this repository:

* `github.com/infogulch/xtemplate`, a library that loads template files and implements `http.Handler`, routing requests to templates. Use it in your own Go application by depending on it as a library and using the [`XTemplate` struct](templates.go) struct.
* `github.com/infogulch/xtemplate/bin`, a simple binary that configures `XTemplate` with CLI args and serves http requests with it. Build it yourself or download the binary from github releases.
* `github.com/infogulch/xtemplate/caddy`, a caddy module that integrates xtemplate into the caddy web server.

# Template syntax

Template syntax uses Go's [`html/template`](https://pkg.go.dev/html/template)
module, and extends it with custom functions and useful context.

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
  - `.OriginalReq` is the original, unmodified, un-rewritten request as it
    originally came in over the wire. Has the same fields as `.Req`.
  - `.Params` is a list of path parameters extracted from the url based on the
    current route, see [custom routes](#file-based-routing--custom-routes). `{{.Params.ByName "id"}}`
  - `.RemoteIP` is the client's IP address. `{{.RemoteIP}}`
  - `.Host` is the hostname portion (no port) of the Host header of the HTTP request.
  - `.Cookie` Gets the value of a cookie by name. `{{.Cookie "cookiename"}}`
  - `.Placeholder` gets a caddy "placeholder variable". The braces (`{}`)
    have to be omitted.
  - `.RespStatus` Set the status code of the current response. `{{.RespStatus 201}}`
  - `.RespHeader.Add` Adds a header field to the HTTP response. `{{.RespHeader.Add "Field-Name" "val"}}`
  - `.RespHeader.Set` Sets a header field on the HTTP response, replacing any existing value.
  - `.RespHeader.Del` Deletes a header field on the HTTP response.
- File related funcs. File operations are rooted at the directory specified by the `context_root` config option.
  - `.ReadFile` reads and returns the contents of a file, as-is. Note that the contents are NOT escaped, so you should only read trusted files.
  - `.ListFiles` returns a list of the files in the given directory.
  - `.FileExists` returns true if filename can be opened successfully.
  - `.StatFile` returns Stat of a filename.
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

There are built-in functions that perform actions that are unrelated to a
specific request. See [funcs.go](funcs.go) for details.

#### Go stdlib template functions

See [text/template#Functions](https://pkg.go.dev/text/template#hdr-Functions).

<details>
<summary>
Expand for a stdlib funcs documentation.
</summary>

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

#### Sprig library template functions

See the Sprig documentation for details: [Sprig Function Documentation](https://masterminds.github.io/sprig/).

<details>
<summary>
Expand for a listing of Sprig funcs.
</summary>

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

#### xtemplate functions

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
