`caddy-xtemplates` is a [Caddy](https://caddyserver.com) module that extends
Go's [`html/template` library](https://pkg.go.dev/html/template) to be capable
enough to host an entire server-side application in it. Designed with the
[htmx.org](https://htmx.org/) js library in mind, to make server side apps feel
as interactive as a SPA.

## Features

#### Query the database directly within template definitions

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

#### Define templates and import content from other files

```html
<!DOCTYPE html>
<html>

<title>Home</title>
{{template "/shared/_head.html" .}} <!-- import the contents of a file -->

<body>
    {{template "navbar" .}} <!-- invoke a custom template defined anywhere -->
    ...
</body>
</html>
```

#### Automatic reload

Templates are reloaded and validated automatically as soon as they are modified,
no need to restart the server. If there's a syntax error it keeps the old
version and prints the error out in Caddy's logs.

> Ctrl+S > Alt+Tab > F5

#### File-based routing & custom routes

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
the `.Param` key.

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

## Example

> ***See the todos example repository that exercises most features:***
> https://github.com/infogulch/todos

## Quickstart

Download caddy with all standard modules, plus the `xtemplates` module (!important)
from Caddy's build and download server:

https://caddyserver.com/download?package=github.com%2Finfogulch%2Fcaddy-xtemplates

Write your caddy config and use the xtemplates http handler:

```
:8080

route {
    xtemplates {
        root templates
    }
}
```

Write `.html` files in the root directory specified in your Caddy config.

Run caddy with your config: `caddy run --config Caddyfile`

> Remember Caddy is a super http server, check out the caddy docs for features
> you may want to layer on top. Examples: serving static files (css/js libs), set
> up an auth proxy, caching, rate limiting, automatic https, and more!

## Config

The `xtemplates` caddy config has three options:

```
xtemplates {
    root <root directory where template files are loaded>
    delimiters <left> <right>              # defaults: {{ and }}
    database <driver> <connection string>  # passed unmodified to sql.Open,
}
```

The currently imported drivers are:

* [mattn/sqlite3](https://pkg.go.dev/github.com/mattn/go-sqlite3#section-readme) (requires building with `CGO_ENABLED=1`, not available from the caddy build server)
* [cznic/sqlite](https://pkg.go.dev/modernc.org/sqlite?utm_source=godoc) (available from the caddy build server)

## Development

Install [`xcaddy`](https://github.com/caddyserver/xcaddy), then build:

```sh
# build a caddy executable with the latest version of caddy-xtemplates on github:
xcaddy build --with github.com/infogulch/caddy-xtemplates

# build a caddy executable and override the xtemplates module with your
# modifications in the current directory:
xcaddy build --with github.com/infogulch/caddy-xtemplates=.

# build with CGO in order to use the sqlite3 db driver
CGO_ENABLED=1 xcaddy build --with github.com/infogulch/caddy-xtemplates
```
