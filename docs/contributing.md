# Contributing

<!-- symlinked /CONTRIBUTING.md → /docs/contributing.md -->

## Repository structure

| Package | Description |
|---|---|
| `github.com/infogulch/xtemplate` | Core library: Server, Instance, builtin providers |
| `./app/` | `app.Main`; configures and starts xtemplate from CLI args and JSON config |
| `./app/*/` | App variants (watchfs / git) that reload when files change or a remote updates |
| `./cmd/` | Standalone binary that calls `app.Main`; start here for a custom build |
| `./cmd/*/` | Standalone binaries for the respective app variants |
| `./caddy/` | Caddy module; xtemplate as HTTP middleware |
| `./providers/*/` | Core provider packages (self-register provider types) |
| `./test/` | Main integration test suite |
| `./examples/*/` | Running example apps and integration tests |

## Suggested reading paths

Reference the [glossary](reference/glossary.md) for xtemplate terminology.

### Application startup

- [`cmd/main.go`](/cmd/main.go) - Package main; binary feature selection; calls `app.Main`
- [`app/app.go`](/app/app.go) - Parses cli & json configs and runs `Server.Serve`
- [`config.go`](/config.go) - xtemplate's configuration struct
- [`server.go`](/server.go) - Builds and manages instances
- [`instance.go`](/instance.go) - Immutable instance that responds to HTTP requests
- [`build.go`](/build.go) - Builds an instance from config

### HTTP request handling

- [`server.go:Server.ServeHTTP`](/server.go) - Server HTTP handler, dispatches to the current instance handler
- [`instance.go:Instance.ServeHTTP`](/instance.go) - Instance request handler
- [`handlers.go`](/handlers.go) - Individual handlers for buffered template rendering, static files & content negotiation, and flushing template handlers.

### Builtin providers (dot context)

- [`dot.go`](/dot.go) - `Provider` and extension interfaces
- [`dot_instance.go`](/dot_instance.go) - `.X` (instance)
- [`dot_req.go`](/dot_req.go) - `.Req`
- [`dot_resp.go`](/dot_resp.go) - `.Resp` (buffered handlers)
- [`dot_flush.go`](/dot_flush.go) - `.Flush` (flushing handlers)

### Provider authoring

- [`dot.go`](/dot.go) - provider interfaces (see Builtin providers above)
- [`providers.go`](/providers.go) - Provider type registry (`Register`, `resolveProviders`)
- [`providers/dotsql/sql.go`](/providers/dotsql/sql.go) - Core SQL provider package
- [`examples/dotprovider/`](/examples/dotprovider/) - Custom provider example

## Docs style

Do not hard-wrap prose. One paragraph (or list item) per line; headings, tables, and fenced code stay as-is. GitHub reflows the rendered view.

## Development setup

Requires [mise](https://mise.jdx.dev/).

```shell
mise install  # installs tool versions pinned in .config/mise/config.toml (Go, hurl, Nushell, xcaddy)
```

## Test and build with mise

```shell
mise test             # run all 12 lint/test tasks. ~2:30m clean, ~30s typical
mise ci               # full pipeline: test (above) and build dist

# run a specific suite
mise run gotest       # run all Go unit tests ~1s
mise run test-docker  # build and test the Docker image

# view all available tasks:
mise tasks
```

## CI pipeline

Github Actions runner executes `mise ci` with some caching and artifact upload helper actions.
