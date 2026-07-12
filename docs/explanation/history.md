# Project history

Narrative history of the project. See `CHANGELOG.md` for per-release detail.

## go-htmx

The idea for this project started as [infogulch/go-htmx][go-htmx] (now
archived), which included the first implementations of template-name-based
routing, exposing sql db functionality to templates, and a persistent templates
instance shared across requests and reloaded when template files changed.

## caddy-xtemplate

go-htmx was refactored and rebased on top of the [templates module from the
Caddy server][caddyhttp-templates] to create `caddy-xtemplate` to add some extra
features including reading files directly and built-in funcs for markdown
conversion, and to get a jump start on supporting the broad array of web server
features without having to implement them from scratch.

## xtemplate

xtemplate was then refactored to be usable independently from Caddy. Caddy
support is published as the `caddy` subpackage, which uses the public xtemplate
Go API to integrate xtemplate into Caddy as HTTP middleware.

[go-htmx]: https://github.com/infogulch/go-htmx
[caddyhttp-templates]: https://github.com/caddyserver/caddy/tree/master/modules/caddyhttp/templates
