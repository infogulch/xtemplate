# Glossary

<!-- symlinked /CONTEXT.md → /docs/reference/glossary.md -->

Domain terms for xtemplate. Behavior and APIs live in the rest of the [docs](../README.md) and [Design](../explanation/design.md).

## Lifecycle

**Server**: Long-lived request handler that owns the current instance and can reload it.

**Instance**: Immutable snapshot of a loaded template root (exposed as `.X`). Built at startup or on reload; never mutated in place.

**Reload**: Build a new instance from the same config and swap it in atomically. In-flight requests finish on the old instance.

**Template root**: Directory (`fs.FS`) of templates and static files that defines the app.

## Templates

**Path template**: Named by its path under the template root (default extension `.html`). Implies a `GET` route with the extension stripped.

**Define template**: Named by `{{define "..."}}`. A name that is a method + path pattern is a route; otherwise it is a reusable template. _Avoid_: partial, sub-template, template block

**Static file**: Non-template file under the template root, served as-is (identity plus optional precompressed siblings sharing a content checksum). _Avoid_: asset, resource

**Initialization template**: A defined template whose name starts with `INIT `; run once at instance build, not as a route. Output discarded; error aborts the build. _Avoid_: template initializer, INIT template

**Dot context** (dot): Per-request value of `.`, assembled from dot providers. Sole channel for request data, response control, and data sources.

**Template function**: Stateless FuncMap entry (`{{myFunc x}}`). Defaults: Go builtins, Sprig, xtemplate additions. Not request-scoped (contrast: providers).

**Early return**: Deliberate successful halt mid-template (`return`, `.Resp.ReturnStatus`, some `.Flush` helpers). Not an error. _Avoid_: abort, halt, ReturnError (as the user-facing name)

## Routing

**Route**: Method + path pattern (`http.ServeMux`) on the instance router. From path templates or define-template names; pseudo-method `SSE` selects a flushing handler.

**Buffered handler**: Default template handler; buffers output so status and headers can change mid-execution.

**Flushing handler**: Streaming template handler for `SSE` routes; uses `.Flush`.

## Providers

**Dot provider**: Contributes one named field on the per-request dot (`{{.Field}}`): field name, prototype type, optional init/finalize/close, and per-request value. Implements `Provider`; optional hooks via `Initializer`, `Finalizer`, `Closer`.

**Dot field**: That field's name (`FieldName`); unique per instance.

**Provider type**: Registry key / JSON `"type"` that selects the constructor.

**Builtin provider**: Supplied by xtemplate: `.X` and `.Req` always; `.Resp` on buffered handlers; `.Flush` on flushing handlers.

**Core provider**: Package under `xtemplate/providers` (`sql`, `fs`, `flags`, `bus`, `nats`, `smtp`).

**Standard provider set**: Types included in release binaries (`cmd/*`). For Caddy, blank-import `xtemplate/caddy/standard`.

**Provider package**: Go package that self-registers a provider type in `init()`.

**Custom provider**: User Go code via `Config.Providers` / `WithProvider`, not via the type registry.

**Provider config**: Type, field name, and type-specific settings (JSON via the registry, or a constructed `Provider`).

**Caddyfile provider**: Caddy module `xtemplate.providers.*` that parses `provider <type> <field> { }` into config JSON.
