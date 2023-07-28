`caddy-xtemplate` is a [Caddy](https://caddyserver.com) module that extends
Go's [`html/template` library](https://pkg.go.dev/html/template) to be capable
enough to host an entire server-side application in it. Designed with the
[htmx.org](https://htmx.org/) js library in mind, which makes server side
rendered sites feel as interactive as a Single Page Apps.

> ⚠️ This project is in active development, expect regular breaking changes. ⚠️

> ---
> ### Table of contents
> * ✨ [Features](#features)
>   * [Query the database directly within template definitions](#query-the-database-directly-within-template-definitions)
>   * [Define templates and import content from other files](#define-templates-and-import-content-from-other-files)
>   * [File-based routing & custom routes](#file-based-routing--custom-routes)
>   * [Automatic reload](#automatic-reload)
> * 🐾 [Example](#example)
> * 🏃 [Quickstart](#quickstart)
> * 👓 [Config](#config)
> * 💼 [Template syntax](#template-syntax)
>   * [Context values](#context-values)
>   * [Functions](#functions)
> * 🛠️ [Development](#development)
> * ✅ [License](#project-lineage-and-license)
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
{{template "/shared/_head.html" .}} <!-- import the contents of a file -->

<body>
    {{template "navbar" .}} <!-- invoke a custom template defined anywhere -->
    ...
</body>
</html>
```

### File-based routing & custom routes

`GET` requests for any file will invoke the template file at that path. Except
files that start with `_` which are not routed, this lets you define templates
that only other templates can invoke.

```
.
├── index.html          GET /
├── todos.html          GET /todos
├── admin
│   └── settings.html   GET /admin/settings
└── shared
    └── _head.html      (not routed)
```

Create custom route handlers by defining a template with a name matching the
pattern `<method> <path>`. Use
[httprouter](https://github.com/julienschmidt/httprouter) syntax for path
parameters and wildcards, which are made available in the template as values in
the `.Param` key while serving a request.

```html
{{define "GET /contact/:id"}} <!-- match on path parameters -->
{{$contact := .QueryRow `SELECT name,phone FROM contacts WHERE id=?` (.Params.ByName "id")}}
<div>
    <span>Name: {{.name}}</span>
    <span>Phone: {{.phone}}</span>
</div>
{{end}}

{{define "DELETE /contact/:id"}} <!-- match on any http method -->
{{$_ := .Exec `DELETE from contacts WHERE id=?`  (.Params.ByName "id")}}
OK
{{end}}
```

### Automatic reload

Templates are reloaded and validated automatically as soon as they are modified,
no need to restart the server. If there's a syntax error it continues to serve
the old version and prints the loading error out in Caddy's logs.

> Ctrl+S > Alt+Tab > F5

# Example

> ***See the todos example repository that exercises most features:***
> https://github.com/infogulch/todos

# Quickstart

Download caddy with all standard modules, plus the `xtemplate` module (!important)
from Caddy's build and download server:

https://caddyserver.com/download?package=github.com%2Finfogulch%2Fcaddy-xtemplate

Write your caddy config and use the xtemplate http handler:

```
:8080

route {
    xtemplate {
        root templates
    }
}
```

Write `.html` files in the root directory specified in your Caddy config.

Run caddy with your config: `caddy run --config Caddyfile`

> Remember Caddy is a super http server, check out the caddy docs for features
> you may want to layer on top. Examples: serving static files (css/js libs), set
> up an auth proxy, caching, rate limiting, automatic https, and more!

# Config

The `xtemplate` caddy config has three options:

```
xtemplate {
    root <root directory where template files are loaded>
    delimiters <left> <right>        # defaults: {{ and }}
    database {                       # default empty, no db available
        driver <driver>              # driver and connstr are passed directly to sql.Open
        connstr <connection string>  # check your sql driver for connstr details
    }
}
```

These sql drivers are currently imported (see [db.go](db.go)):

* [mattn/sqlite3](https://pkg.go.dev/github.com/mattn/go-sqlite3#section-readme) (requires building with `CGO_ENABLED=1`, not available from the caddy build server)
* [cznic/sqlite](https://pkg.go.dev/modernc.org/sqlite?utm_source=godoc) (available from the caddy build server)

# Template syntax

Template syntax uses Go's [`html/template`](https://pkg.go.dev/html/template)
module, and extends it with custom functions and useful context.

### Context values

The dot context `{{.}}` set on the main template handler provides
request-specific data and stateful actions. See [tplcontext.go](tpl.context.go)
for details.

* Request and response related fields and fields
    * `.Req` is the current HTTP request struct, [http.Request](https://pkg.go.dev/net/http#Request), which has various fields, including:
        - `.Method` - the method
        - `.URL` - the URL, which in turn has component fields (Scheme, Host,
          Path, etc.)
        - `.Header` - the header fields
        - `.Host` - the Host or :authority header of the request
    * `.OriginalReq` is the original, unmodified, un-rewritten request as it
    originally came in over the wire. Has the same fields as `.Req`.
    * `.Params` is a list of path parameters extracted from the url based on the
    current route, see [custom routes](#file-based-routing--custom-routes). `{{.Params.ByName "id"}}`
    * `.RemoteIP` is the client's IP address. `{{.RemoteIP}}`
    * `.Host` is the hostname portion (no port) of the Host header of the HTTP request.
    * `.Cookie` Gets the value of a cookie by name. `{{.Cookie "cookiename"}}`
    * `.Placeholder` gets a caddy "placeholder variable". The braces (`{}`)
      have to be omitted.
    * `.RespStatus` Set the status code of the current response. `{{.RespStatus 201}}`
    * `.RespHeader.Add` Adds a header field to the HTTP response. `{{.RespHeader.Add "Field-Name" "val"}}`
    * `.RespHeader.Set` Sets a header field on the HTTP response, replacing any existing value.
    * `.RespHeader.Del` Deletes a header field on the HTTP response.
* File related funcs. File operations are rooted at the directory specified by the `root` config option.
    * `.ReadFile` reads and returns the contents of another file, as-is. Note that the contents are NOT escaped, so you should only read trusted files.
    * `.ListFiles` returns a list of the files in the given directory, which is relative to the template context's file root.
    * `.FileExists` returns true if filename can be opened successfully
    * `.StatFile` returns Stat of a filename.
* Database related funcs. All funcs accept a query string and any number of parameters. Prefer using parameters over building the query string dynamically.
    * `.Exec` executes a statment
    * `.QueryRows` executes a query and returns all rows in a big `[]map[string]any`.
    * `.QueryRow` executes a query, which must return one row, and returns the `map[string]any`.
    * `.QueryVal` executes a query, which must return one row and one column, and returns the value of the column.
* Other
    * `.Template` evaluate the template name with the given context and return the result as a string.

### Functions

There are built-in functions that perform actions that are unrelated to a
specific request. See [funcs.go](funcs.go) for details.

* The standard Go `text/template` functions, such as `and`, `index`, `slice`, `print`. See [text/template#Functions](https://pkg.go.dev/text/template#hdr-Functions).
* [All Sprig functions](https://masterminds.github.io/sprig/), such as: `list`, `dict`, `trim`, `atoi`, `uuidv4`, `env`, `fail`, `b64enc`, `now`, `date`
* `markdown` Renders the given Markdown text as HTML and returns it. This uses the [Goldmark](https://github.com/yuin/goldmark) library, which is CommonMark compliant. It also has these extensions enabled: Github Flavored Markdown, Footnote, and syntax highlighting provided by [Chroma](https://github.com/alecthomas/chroma).
* `splitFrontMatter` Splits front matter out from the body. Front matter is metadata that appears at the very beginning of a file or string. Front matter can be in YAML, TOML, or JSON formats.
    * `.Meta` to access the metadata fields, for example: `{{$parsed.Meta.title}}`
    * `.Body` to access the body after the front matter, for example: `{{markdown $parsed.Body}}`
* `stripHTML` Removes HTML from a string. `{{stripHTML "Shows <b>only</b> text content"}}`
* `humanize` Transforms size and time inputs to a human readable format using the [go-humanize](https://github.com/dustin/go-humanize) library. Call with two parameters, the format type and the value to format. Format types are:
    - **size** which turns an integer amount of bytes into a string like `2.3 MB`, for example: `{{humanize "size" "2048000"}}`
    - **time** which turns a time string into a relative time string like `2 weeks ago`, for example: `{{humanize "time" "Fri, 05 May 2022 15:04:05 +0200"}}`
* `uuid` returns a RFC 4122 UUID using [google/uuid](https://github.com/google/uuid)
* `ksuid` returns a 'K-Sortable Globally Unique ID' using [segmentio/ksuid](https://github.com/segmentio/ksuid)
* `idx` gets an item from a list, similar to the built-in `index`, but with reversed args: index first, then array. This is useful to use index in a pipeline, for example: `{{generate-list | idx 5}}`

# Development

To work on this project, install [`xcaddy`](https://github.com/caddyserver/xcaddy), then build from the repo root:

```sh
# build a caddy executable with the latest version of caddy-xtemplate from github:
xcaddy build --with github.com/infogulch/caddy-xtemplate

# build a caddy executable and override the xtemplate module with your
# modifications in the current directory:
xcaddy build --with modulepath=.

# build with CGO in order to use the sqlite3 db driver
CGO_ENABLED=1 xcaddy build --with github.com/infogulch/caddy-xtemplate
```

## Project lineage and license

This project is based on and shares some code with the [templates module from
the Caddy server][1], and is also licensed under the Apache 2.0 license. See
[LICENSE](./LICENSE)

[1]: https://github.com/caddyserver/caddy/tree/master/modules/caddyhttp/templates
