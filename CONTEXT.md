# xtemplate

A server-side HTML templating engine that extends `html/template` with a per-request dot context populated by pluggable data providers, so a directory of templates can define an entire application.

## Language

### Server & lifecycle

**Server**:
The long-lived, reloadable request handler that owns the current instance and routes incoming requests to it. One server exists per running xtemplate and persists, while the instance it points to may be replaced.

**Reload**:
A server operation that builds a fresh instance from the same config (re-reading the template root, re-parsing templates, re-initializing providers) and atomically swaps it in as the current instance. In-flight requests finish on the old instance, new requests use the new one, and the old instance's context is cancelled so there's no process restart and no dropped requests.

**Instance**:
A fully-loaded, immutable snapshot of a template root that serves HTTP requests. Instances are built once at startup (or on reload) and swapped atomically; a running instance is never mutated. The instance itself is exposed to templates as the `.X` dot field.

**Template root**:
The directory of template files that defines an application, backed by an `fs.FS`. Loaded recursively at startup; every `.html` file becomes invocable by its path relative to this root (e.g. `/admin/settings.html`), and files map to routes based on their path.

### Templates & execution

**Template**:
A single named, parsed unit in an instance's template namespace. Every template gets its name one of two ways, and the name determines how it can be reached:

- **Path template** — named after its path in the template root (e.g. `/admin/settings.html`), parsed from the whole content of a file matching the configured extension (default `.html`). Its path derives an implicit `GET` route.
- **Define template** — named by an explicit `{{define "..."}}` block. If the name matches a method + path pattern (e.g. `POST /contact`), the name itself is the route; otherwise it is a reusable template invoked by name from other templates.

_Avoid_: partial, sub-template, template block, template file, named template definition

**Initialization template**:
A template whose name begins with `INIT ` that xtemplate executes once when an instance is built, before it serves any request, rather than in response to a route. Used for one-time setup such as seeding a database. Its rendered output is discarded, and an error aborts the build.
_Avoid_: template initializer, INIT template

**Dot context** (or **dot**):
The single value passed as `.` to every template execution for a request. It is a struct assembled per-request whose fields are contributed by dot providers (builtin ones like `.X`, `.Req`, `.Resp`, `.Flush`, plus any configured or custom providers). By design it is the sole channel through which templates reach request data, response control, and backing data sources; FuncMap functions are non-request-scoped.

**Template function**:
A function callable from any template (`{{myFunc x}}`), supplied via a FuncMap at startup. Unlike dot providers, functions are stateless and intended for pure computation (formatting, string/data manipulation). Three sets are present by default: Go's builtins, the Sprig library, and xtemplate's own additions.

**Early return**:
A successful, deliberate halt of a template's execution partway through, as opposed to an error. Triggered by the `return` function and by dot methods such as `.Resp.ReturnStatus` and `.Flush.Sleep`; internally signalled by the `ReturnError` sentinel, which handlers treat as normal completion rather than a failure.
_Avoid_: abort, halt, ReturnError (as the user-facing name)

### Routing

**Route**:
A method + path pattern (in `http.ServeMux` syntax) mapped to a handler on an instance's router. Routes come from two sources: path templates, whose file path becomes the pattern (e.g. `admin/settings.html` → `GET /admin/settings`; `index.html` maps to its directory; dotfiles are excluded), and define templates whose name is itself an explicit method + pattern, e.g. `{{define "GET /contact/{id}"}}`. The pseudo-method `SSE` selects a flushing handler.

**Handler**:
The `http.Handler` that answers a matched route. An instance's router mixes four kinds: **buffered** template handlers (the default; output is buffered so `.Resp` can set status/headers mid-execution and errors abort cleanly), **flushing** template handlers (stream incrementally via `.Flush` for Server-Sent Events; selected by the `SSE` verb in a route name), **static file** handlers (serve non-template files from the template root), and **custom** handlers (user-supplied handlers mounted by pattern).

### Providers

**Dot provider**:
A component that contributes one named field to the per-request template dot context (`{{.Field}}`). Each dot provider has a field name, a one-time init step, and a per-request value step.

**Dot field**:
The named struct field a dot provider contributes to the dot context. Its name is chosen by the user (the provider's `FieldName`); two providers may not share a field name within one instance.

**Provider type**:
The registered name of a provider and the discriminator string in provider JSON (`"type"`) that selects which dot provider constructor to invoke from the registry.

**Builtin provider**:
A dot provider supplied by xtemplate itself and present by default on relevant requests. Currently: `X` (instance) and `Req` (request) are provided on every request, `Resp` available on buffered handlers, and `Flush` is available on flushing handlers.

**Provider package**:
Any Go package that self-registers a dot provider type via `init()`.

**Core provider**:
Provider packages published by xtemplate under `xtemplate/providers`. Currently: `sql`, `fs`, `flags`, `nats`.

**Standard provider set**:
The set of provider types that are included by default in xtemplate's binary packages. Currently all core providers are included in the binaries under `cmd`. For caddy builds the standard provider set is available by blank-importing `xtemplate/caddy/standard`.

**Custom provider**:
A dot provider implemented in user Go code and attached to an instance in the `Config.Providers` slice or via `WithProvider`, not appearing in the type registry.

**Provider config**:
The JSON or Go-API configuration of a dot provider instance: its type, its field name, and its type-specific settings. JSON configs arrive as raw messages and are decoded via the registry; custom provider configs arrive as already-constructed `DotConfig` values.

**Caddyfile provider**:
A Caddy module in the `xtemplate.providers.*` namespace that implements the `CaddyfileProvider` interface and parses a Caddyfile `provider <type> <field> { }` block into a `json.RawMessage`.
