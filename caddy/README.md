# `xtemplate_caddy` module

`xtemplate_caddy` adapts [`xtemplate`](https://github.com/infogulch/xtemplate)
for use in the [Caddy](https://caddyserver.com/) web server by:

1. Publishing as a caddy module named [`http.handlers.xtemplate`](https://caddyserver.com/download?package=github.com%2Finfogulch%2Fxtemplate%2Fcaddy)
2. Exposing it as a `caddyhttp.MiddlewareHandler` that can serve as a route
   handler using the `xtemplate` handler middleware definition.
3. Adapts Caddyfile configuration to `XTemplate` config.

> See [xtemplate readme](../README.md) for syntax details.

## Quickstart

Download caddy with all standard modules, plus the `xtemplate` module (!important)
from Caddy's build and download server:

https://caddyserver.com/download?package=github.com%2Finfogulch%2Fxtemplate

Write your caddy config and use the xtemplate http handler:

```
:8080

route {
    xtemplate {
        template_root templates
    }
}
```

Write `.html` files in the root directory specified in your Caddy config.

Run caddy with your config: `caddy run --config Caddyfile`

> Caddy is a very capable http server, check out the caddy docs for features
> you may want to layer on top. Examples: serving static files (css/js libs), set
> up an auth proxy, caching, rate limiting, automatic https, etc

## Config

Here are the xtemplate configs available to a Caddyfile:

```
xtemplate {
    template_root <root directory where template files are loaded>
    context_root <root directory that template funcs have access to>
    delimiters <left> <right>         # defaults: {{ and }}
    database {                        # default empty, no db available
        driver <driver>               # driver and connstr are passed directly to sql.Open
        connstr <connection string>   # check your sql driver for connstr details
    }
    config {                          # a map of configs, accessible in the template as .Config
      key1 value1
      key2 value2
    }
    funcs_modules <mod1> <mod2>       # a list of caddy modules under the `xtemplate.funcs.*`
                                      # namespace that implement the FuncsProvider interface,
                                      # to add custom funcs to the Template FuncMap.
}
```

## Build

To build xtemplate_caddy locally, install [`xcaddy`](https://github.com/caddyserver/xcaddy), then build from the directory root:

```sh
# build a caddy executable with the latest version of xtemplate from github:
xcaddy build --with github.com/infogulch/xtemplate

# build a caddy executable and override the xtemplate module with your
# modifications in the current directory:
xcaddy build --with github.com/infogulch/xtemplate=.

# build with CGO in order to use the sqlite3 db driver
CGO_ENABLED=1 xcaddy build --with github.com/infogulch/xtemplate

# build enable the sqlite_json build tag to get json funcs
GOFLAGS='-tags="sqlite_json"' CGO_ENABLED=1 xcaddy build --with github.com/infogulch/xtemplate

TZ=UTC git --no-pager show --quiet --abbrev=12 --date='format-local:%Y%m%d%H%M%S' --format="%cd-%h"
```
