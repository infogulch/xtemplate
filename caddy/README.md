# xtemplate/caddy

xtemplate/caddy adapts [xtemplate][xtemplate] for use in the [Caddy][caddy] web server by:

1. Registering as a [Caddy module][extending-caddy] named [`http.handlers.xtemplate`][http.handlers.xtemplate] which exposes a `caddyhttp.MiddlewareHandler` that can serve as a route handler using the `xtemplate` handler middleware definition.
2. Adapting Caddyfile configuration to easily configure xtemplate through Caddy's configuration system.

[xtemplate]: https://github.com/infogulch/xtemplate
[caddy]: https://caddyserver.com/
[extending-caddy]: https://caddyserver.com/docs/extending-caddy
[http.handlers.xtemplate]: https://caddyserver.com/download?package=github.com%2Finfogulch%2Fxtemplate%2Fcaddy%2Fstandard

## Quickstart

First, [Download Caddy with `http.handlers.xtemplate` (standard: providers + sources + sqlite3)][http.handlers.xtemplate], or [build it yourself](#build).

Write your caddy config and use the xtemplate http handler in a route block. See [Config](#config) for a listing of xtemplate configs. The simplest Caddy config is:

```Caddy
:8080

route {
    xtemplate
}
```

Without a `source` block, templates load from `./templates` via the default **os** source (no filesystem watch). Place `.html` files there relative to the process working directory.

Run caddy with your config:

```shell
caddy run --config Caddyfile
```

> [!TIP]
> Caddy is a very capable http server, check out the [Caddy docs](https://caddyserver.com/docs) for features you may want to layer on top. Examples: set up an auth proxy, caching, rate limiting, automatic https, etc.

> [!IMPORTANT]
> **Caddy no longer watches templates by default.** Use `source watchfs { … }` for reload-on-change. Legacy directives `templates_dir` / `templates_path` / `watch_template_path` hard-reject with migrate errors (pre-1.0).

## Config

Here are the xtemplate configs available to a Caddyfile:

```Caddy
xtemplate {
    source <type> {                      # optional; default is os path "templates"
        # type-specific options
    }
    template_extension <string>          # File extension for templates. Default ".html".
    minify <bool>                        # Minify html templates at load time. Default: true.
    delimiters <Left:string> <Right:string>  # Template action delimiters, default "{{" and "}}".
    precompress <enc...>                 # Generate static encodings at load: gzip, zstd, br (repeatable).

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

### Source blocks

```Caddy
source os {
    path templates
}

source watchfs {
    path templates
    watch data          # extra dirs (optional, repeatable via multiple args)
    debounce 200ms
}

source git {
    repo https://example.com/site.git
    ref main
    interval 15s
    path templates      # subdir inside clone
}
```

Built-in `os` is registered by this package. `watchfs` / `git` require linking their `sources/*/caddyfile` modules (`caddy/standard` includes them).

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
    path <dir>
    # writable true   # enables ReceiveFiles
}
```

See provider packages under `providers/*/caddyfile` for flags, bus, nats, smtp.

## Build

```shell
xcaddy build --with github.com/infogulch/xtemplate/caddy/standard
```

Or select individual provider/source caddyfile packages with additional `--with` lines.
