# xtemplate/caddy

xtemplate/caddy adapts [xtemplate][xtemplate] for use in the [Caddy][caddy] web server by:

1. Registering as a [Caddy module][extending-caddy] named [`http.handlers.xtemplate`][http.handlers.xtemplate] which exposes a `caddyhttp.MiddlewareHandler` that can serve as a route handler using the `xtemplate` handler middleware definition.
2. Adapting Caddyfile configuration to easily configure xtemplate through Caddy's configuration system.

[xtemplate]: https://github.com/infogulch/xtemplate
[caddy]: https://caddyserver.com/
[extending-caddy]: https://caddyserver.com/docs/extending-caddy
[http.handlers.xtemplate]: https://caddyserver.com/download?package=github.com%2Finfogulch%2Fxtemplate%2Fcaddy%2Fstandard

## Quickstart

First, [Download Caddy with `http.handlers.xtemplate` (standard: providers + sqlite3)][http.handlers.xtemplate], or [build it yourself](#build).

Write your caddy config and use the xtemplate http handler in a route block. See [Config](#config) for a listing of xtemplate configs. The simplest Caddy config is:

```Caddy
:8080

route {
    xtemplate
}
```

Place `.html` files in the directory specified by the `templates_dir` option in your caddy config (default "templates"). The config above would load templates from the `./templates` directory, relative to the current working directory.

Run caddy with your config:

```shell
caddy run --config Caddyfile
```

> [!TIP]
> Caddy is a very capable http server, check out the [Caddy docs](https://caddyserver.com/docs) for features you may want to layer on top. Examples: set up an auth proxy, caching, rate limiting, automatic https, etc.

## Config

Here are the xtemplate configs available to a Caddyfile:

```Caddy
xtemplate {
    templates_dir <string>                   # The path to the templates directory. Default: "templates".
    watch_template_path <bool>               # Reload templates if anything in templates_dir changes. Default: true
    template_extension <string>              # File extension to search for to find template files. Default ".html".
    minify <bool>                            # Minify html templates at load time. Default: true.
    delimiters <Left:string> <Right:string>  # The template action delimiters, default "{{" and "}}".
    precompress <enc...>                     # Generate static encodings at load: gzip, zstd, br (repeatable).

    crossorigin {
        disabled <bool>                      # Disable Go 1.25 cross-origin (CSRF) protection. Default: false.
        trusted_origins <origin...>          # Origins allowed to make unsafe cross-origin requests.
        insecure_bypass_patterns <pattern...> # Request path patterns exempt from cross-origin protection.
    }

    provider <type> <field> {
        # provider-specific options (see Provider blocks below)
    }
}
```

### Provider blocks

Dot providers are configured with `provider <type> <field> { }` blocks. The `<type>` selects the provider kind and `<field>` sets the dot field name templates use to access it (e.g. `.DB`, `.FS`).

Caddyfile syntax covers a curated subset of each provider's options. For advanced configuration use Caddy's JSON format ([caddy.json](../test/caddy.json) has examples); all JSON fields remain available as a full-fidelity escape hatch.

**sql**: connect to a SQL database (`caddy/standard` links pure-Go `sqlite3`; other drivers need a blank import in a custom build):

```Caddy
provider sql DB {
    driver   <driver>   # e.g. sqlite3, pgx, mysql
    connstr  <connstr>  # driver-specific connection string
    max_open_conns <n>  # optional connection pool limit
}
```

**fs**: expose a directory as a read/write filesystem:

```Caddy
provider fs FS {
    path <dir>  # root directory path
}
```

**flags**: static key/value pairs accessible in templates:

```Caddy
provider flags Flags {
    env        production
    version    1.2.3
}
```

**nats**: connect to a NATS messaging server:

```Caddy
provider nats Nats {
    in_process_server {
        dont_listen true   # embedded server, no external port
    }
    conn_options {
        url nats://localhost:4222
    }
}
```

JetStream options and advanced `server.Options` fields have no JSON representation and must be set via the Go API.

#### Linking providers into the binary

Provider Caddyfile support lives in opt-in subpackages. Use `caddy/standard` to pull in the default set (sql, fs, flags, nats) in one flag, or add one `--with` flag per provider to build a leaner subset.

```shell
# default set (sql + fs + flags + nats Caddyfile + pure-Go sqlite3 driver)
xcaddy build --with github.com/infogulch/xtemplate/caddy/standard

# leaner subset: pick desired providers (and a SQL driver) individually
xcaddy build \
    --with github.com/infogulch/xtemplate/caddy \
    --with github.com/infogulch/xtemplate/providers/dotsql/caddyfile \
    --with github.com/infogulch/xtemplate/providers/dotfs/caddyfile \
    --with github.com/infogulch/xtemplate/providers/dotflags/caddyfile \
    --with github.com/ncruces/go-sqlite3/driver
```

A provider type unknown at parse time produces an actionable error:

```
provider type "sql" is not available in this build;
add it with --with github.com/infogulch/xtemplate/providers/dotsql/caddyfile
```

> `templates_path` is accepted as a deprecated alias for `templates_dir`.

### Custom template functions

You can add custom template functions by writing a Caddy module in the `xtemplate.funcs` namespace that implements the `FuncsProvider` interface (`Funcs() template.FuncMap`), then referencing it by name with the `funcs_modules` option in Caddy's json configuration:

```json
{
    "handler": "xtemplate",
    "funcs_modules": ["myfuncs"]
}
```

This loads the module registered as `xtemplate.funcs.myfuncs` and merges the functions it returns into the template execution context.

## Build

### `xcaddy` CLI

To build with xtemplate locally, install [`xcaddy`](xcaddy), then:

```shell
# default provider set + pure-Go sqlite3 driver
xcaddy build --with github.com/infogulch/xtemplate/caddy/standard
```

No CGO is required for that combination. CGO (and driver-specific build tags) only matter if you choose a CGO-based SQL driver or other CGO modules.

[xcaddy]: https://github.com/caddyserver/xcaddy

<details>

```shell
TZ=UTC git --no-pager show --quiet --abbrev=12 --date='format-local:%Y%m%d%H%M%S' --format="%cd-%h"
```

</details>

### A Go module

Create a go module `go mod init <modname>` with a `main.go` like this:

```go
package main

import (
	caddycmd "github.com/caddyserver/caddy/v2/cmd"

	_ "github.com/infogulch/xtemplate/caddy/standard"
	// caddy/standard already links sqlite3; other drivers:
	// _ "github.com/jackc/pgx/v5/stdlib"

	// Other Caddy modules, e.g.:
	// _ "github.com/greenpau/caddy-security"
)

func main() {
	caddycmd.Main()
}
```

Compile it with `go build -o caddy .`, then run with `./caddy run --config Caddyfile`

## Package history

This package has moved several times. Here are some previous names it has been known as:

* `github.com/infogulch/caddy-xtemplate` - Initial implementation to prove out the idea.
* `github.com/infogulch/xtemplate/caddy` - Refactored xtemplate to be usable from the cli and as a Go library, split Caddy integration into a separate module in the same repo.
* `github.com/infogulch/xtemplate-caddy` - Caddy integration moved to its own repo, and refactored config organization.
* `github.com/infogulch/xtemplate/caddy` - Moved back to the main xtemplate repo as part of the xtemplate module, because I learned that splitting modules doesn't actually help reduce dependencies.
