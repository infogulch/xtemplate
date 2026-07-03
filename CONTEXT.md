# xtemplate

A server-side HTML templating engine that extends `html/template` with a per-request dot context populated by pluggable data providers, so a directory of templates can define an entire application.

## Language

**Dot provider**:
A component that contributes one named field to the per-request template dot context (`{{.Field}}`). Each dot provider has a field name, a one-time init step, and a per-request value step.
_Avoid_: context provider (legacy README term), data source

**Dot field**:
The named struct field a dot provider contributes to the dot context. Its name is chosen by the user (the provider's `FieldName`); two providers may not share a field name within one instance.

**Provider type**:
The discriminator string in provider JSON (`"type"`) that selects which dot provider constructor to invoke from the registry. Each type corresponds to a provider package at `xtemplate/providers/<type>`. Current types: `sql`, `fs`, `flags`, `nats`.
_Avoid_: provider kind, provider category

**Builtin provider**:
A dot provider and field that is always available during a request, regardless of config. Currently: `X` (instance), `Req` (request), `Resp` (response), and `Flush` (flusher).

**Standard provider set**:
The set of provider types enabled by default in xtemplate's binary packages: `sql`, `fs`, `flags`. Go binaries import the provider packages they use explicitly (no aggregator package); caddy builds aggregate the default set's Caddyfile syntax via `xtemplate/caddy/standard`. The name survives as a label for `{sql, fs, flags}`, not as a Go package. A binary that wants a subset imports individual provider packages instead.
_Avoid_: default provider, core provider (obsolete under uniformity)

**Provider package**:
A Go package at `xtemplate/providers/<type>`, outside the main `xtemplate` package, that self-registers one dot provider type via `init()`. Every dot provider lives in a provider package; the `xtemplate` package itself exports only the registry, the `DotConfig` interface, and `WithProvider`. A binary that doesn't import a provider package doesn't link its dependencies. Examples: `xtemplate/providers/sql`, `xtemplate/providers/nats`.
_Avoid_: plugin, extension, core provider (obsolete)

**Custom provider**:
A dot provider implemented in user Go code and registered via `WithProvider`, not appearing in the type registry.
_Avoid_: user provider, ad-hoc provider

**Provider config**:
The JSON or Go-API description of a dot provider instance: its type, its field name, and its type-specific settings. JSON configs arrive as raw messages and are decoded via the registry; Go-API configs arrive as already-constructed `DotConfig` values.

**Provider config decoding**:
The registry peeks the `"type"` discriminator from each JSON provider config to select a constructor, then re-decodes the full raw message into the provider's concrete type. Re-decoding uses Go's default `encoding/json` rules: unknown fields are silently ignored, so the `"type"` key is consumed by the registry and not re-decoded into the provider struct. A typo'd discriminator (e.g. `"type":"fz"`) fails at registry lookup with an "unknown type" error; a typo'd setting key is silently dropped.

**Provider package name aliasing**:
The provider package names (`sql`, `fs`, `nats`) collide with common stdlib and external package names (`database/sql`, `io/fs`, `github.com/nats-io/nats.go`). Convention: the provider package keeps its bare name; the caller aliases the stdlib or external library. Examples: `stdsql "database/sql"`, `stdfs "io/fs"`, `natsgo "github.com/nats-io/nats.go"`. Provider source files themselves need no aliasing — their own package name is never in scope within the package. JSON-only users and blank-import users (`_ "github.com/infogulch/xtemplate/providers/sql"`) are unaffected.

**Caddyfile provider**:
A Caddy module in the `xtemplate.providers.*` namespace that parses a Caddyfile `provider <type> <field> { }` block into a `json.RawMessage` for `ProvidersRaw`. Every provider package contributes its Caddyfile module from a sibling `caddyfile` subpackage (`providers/<type>/caddyfile`) that registers the Caddy module and blank-imports the provider package — bundling the dot-provider constructor with the Caddyfile parser behind one opt-in. `xtemplate/caddy` provides only the dispatch (the `CaddyfileProvider` interface and the `provider` block switch); it has no in-tree parsers and imports no provider packages. The Caddyfile syntax is a curated subset of the provider's JSON surface; JSON remains the full-fidelity escape hatch.
_Avoid_: provider parser, caddyfile adapter
