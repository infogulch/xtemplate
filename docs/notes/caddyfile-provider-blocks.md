# Caddyfile provider blocks for dot providers

Prerequisite: [unified-dot-provider-registry.md](./unified-dot-provider-registry.md). This adds Caddyfile `provider <type> <field> { }` syntax on top of the registry's `ProvidersRaw []json.RawMessage`. The registry consumes only JSON; this change is purely a Caddyfile→JSON adapter layer and touches neither the registry nor the provider packages themselves.

## Context

After the registry move, caddy users configure dot providers via JSON only — the Caddyfile has no provider syntax (`caddy/caddyfile.go` handles only `templates_dir`, `delimiters`, `minify`, `crossorigin`, etc.). This follow-up adds Caddyfile `provider` blocks so caddy users can express the common cases without dropping to JSON, mirroring Caddy's own philosophy — Caddyfile for common options, JSON for advanced, Go API for what JSON cannot express.

## Mechanism

Providers are configurable via Caddyfile `provider <type> <field> { }` blocks. `parseCaddyfile` reads the type and field name from the header, dispatches by type to a Caddyfile provider module — a Caddy module in the `xtemplate.providers.*` namespace implementing `CaddyfileProvider` (`ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error)`) — then injects `"type"` and `"name"` into the returned JSON and appends to `ProvidersRaw`. The provider owns its block grammar and emits properly-typed JSON (bools as bools, ints as ints) for its type-specific fields. The dot-provider registry is uninvolved at Caddyfile time; it only consumes the resulting JSON at Instance time.

Every provider package contributes its Caddyfile syntax from a sibling `caddyfile` subpackage (`providers/<type>/caddyfile`) that registers the Caddy module and blank-imports the provider package — bundling the dot-provider constructor with the Caddyfile parser behind one opt-in. There are no in-tree parsers in `xtemplate/caddy`; it provides only the `CaddyfileProvider` interface and the `provider` block dispatch. `providers/<type>` stays caddy-free; the only package importing both caddy and the provider is the opt-in `providers/<type>/caddyfile`. The default set's Caddyfile parsers are aggregated by `xtemplate/caddy/standard`, which blank-imports `providers/{sql,fs,flags}/caddyfile` — and thereby transitively links `providers/{sql,fs,flags}`, so `caddy/standard` doubles as the one-opt-in standard-set link for caddy users (both Caddyfile and JSON-only) who want `{sql, fs, flags}` without per-provider `--with` lines.

Caddyfile syntax is a curated subset of each provider's JSON surface, not a complete mirror. Nats covers `in_process_server { dont_listen true }` and `conn_options { url ... }`; advanced `server.Options` fields use JSON. Jetstream options are **not** covered by either Caddyfile or JSON — they are `func(*jetstream.Options)` values with no JSON representation, settable only via the Go API (`WithNats(..., jsOpts)`); a caddy user who needs them must use a custom build. The default-set providers use block-only syntax. Inline mode is a documented future enhancement.

## Considered options (Caddyfile dispatch)

- **Single registry + interface assertion on the config type**: look up the provider in the dot-provider registry, assert the returned `DotConfig` has a `ParseBlock(httpcaddyfile.Helper)` method. Rejected: the method lives in the config type's package, forcing every provider package (`providers/sql`, `providers/fs`, `providers/flags`, `providers/nats`) to import Caddy — re-creating the exact coupling the registry removed, in both directions.
- **Core-owned `BlockParser` abstraction**: abstract `httpcaddyfile.Helper` behind a core-owned interface so the config-type method doesn't import Caddy. Rejected: leaks a Caddyfile-shaped concept into the `xtemplate` package (which never reads a Caddyfile), and cramps provider authoring by removing `h.ArgErr()`/`h.Errf()`/nested-block support.
- **Custom caddy-side registry**: a guarded `map[string]func(httpcaddyfile.Helper) (json.RawMessage, error)` with `RegisterCaddyfileProvider`. Rejected in favor of reusing Caddy's module registry, which `xtemplate.funcs.*` already establishes as the dispatch-table pattern in this repo.

## Consequences (Caddyfile)

- `xtemplate/caddy` gains `CaddyfileProvider` interface + `provider` dispatch only. No in-tree parsers, no provider imports. Stays provider-free.
- New opt-in packages `providers/<type>/caddyfile` for every provider (`module.go` for registration + compile-time guard, `caddyfile.go` for parsing). Each blank-imports its `providers/<type>`. `xtemplate/caddy/standard` aggregates the default set.
- caddy + standard set + Caddyfile: `xcaddy build --with .../xtemplate/caddy --with .../xtemplate/caddy/standard`.
- caddy + nats + Caddyfile: add `--with .../providers/nats/caddyfile`.
- caddy JSON users who want the standard set linked in one opt-in (no Caddyfile) can also `--with .../xtemplate/caddy/standard`; its blank-imports pull in the constructors. This retires the registry note's "explicit `--with` per provider for caddy JSON users" friction once this follow-up lands.
- Unknown provider type at parse time: error derives the import path from the `providers/<type>/caddyfile` convention — `"provider type %q is not available in this build; add it with --with github.com/infogulch/xtemplate/providers/%s/caddyfile"`. No hardcoded map.
- `caddy/README.md` documents `provider` block syntax and the curated-subset/JSON escape-hatch, replacing the JSON-only note left by the registry change.

## Implementation

### New file `caddy/caddyfile_provider.go`

`CaddyfileProvider` interface, the `provider` block dispatch in `parseCaddyfile`'s
switch, and nothing else — no in-tree parsers. The dispatch reads the header
with two `h.NextArg()` calls (type, field name), looks up
`caddy.GetModule("xtemplate.providers."+type)`, asserts `CaddyfileProvider`,
calls `ParseCaddyfile(h)`, then injects `"type"` and `"name"` into the returned
JSON via a map round-trip and appends to `m.ProvidersRaw`. The unknown-type
error derives the import path from the `providers/<type>/caddyfile` convention.
The map round-trip errors if the provider parser emitted a `"type"` or `"name"`
key (those keys are reserved for the dispatch to inject; a provider emitting
them is a contract violation, surfaced at parse time rather than silently
overwritten):

```go
type CaddyfileProvider interface {
    ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error)
}

// inside parseCaddyfile's block switch:
case "provider":
    if !h.NextArg() {
        return nil, h.Errf("provider requires a type and field name")
    }
    typeName := h.Val()
    if !h.NextArg() {
        return nil, h.Errf("provider %s requires a field name", typeName)
    }
    fieldName := h.Val()
    mi, err := caddy.GetModule("xtemplate.providers." + typeName)
    if err != nil {
        return nil, h.Errf("provider type %q is not available in this build; "+
            "add it with --with github.com/infogulch/xtemplate/providers/%s/caddyfile",
            typeName, typeName)
    }
    parser, ok := mi.New().(CaddyfileProvider)
    if !ok {
        return nil, h.Errf("module xtemplate.providers.%s does not implement CaddyfileProvider", typeName)
    }
    raw, err := parser.ParseCaddyfile(h)
    if err != nil { return nil, err }
    var m map[string]json.RawMessage
    if err := json.Unmarshal(raw, &m); err != nil {
        return nil, h.Errf("provider %s: parser returned invalid JSON: %w", typeName, err)
    }
    if _, ok := m["type"]; ok {
        return nil, h.Errf("provider %s parser must not emit a 'type' key; it is injected by the dispatch", typeName)
    }
    if _, ok := m["name"]; ok {
        return nil, h.Errf("provider %s parser must not emit a 'name' key; it is injected by the dispatch", typeName)
    }
    m["type"], _ = json.Marshal(typeName)
    m["name"], _ = json.Marshal(fieldName)
    final, err := json.Marshal(m)
    if err != nil { return nil, err }
    t.ProvidersRaw = append(t.ProvidersRaw, final)
```

Each provider's `caddyfile` subpackage owns its block walk (`for h.NextBlock(1)`)
and emits only its type-specific fields (no `type`/`name` — those are injected).
Example (`providers/sql/caddyfile`):

```go
func (sqlCaddyfile) ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error) {
    var driver, connstr string
    var maxOpen int
    for h.NextBlock(1) {
        switch h.Val() {
        case "driver":       if !h.AllArgs(&driver) { return nil, h.ArgErr() }
        case "connstr":      if !h.AllArgs(&connstr) { return nil, h.ArgErr() }
        case "max_open_conns":
            var s string
            if !h.AllArgs(&s) { return nil, h.ArgErr() }
            n, err := strconv.Atoi(s)
            if err != nil { return nil, h.Errf("max_open_conns must be int: %v", err) }
            maxOpen = n
        default: return nil, h.Errf("unknown sql provider option '%s'", h.Val())
        }
    }
    return json.Marshal(struct {
        Driver string `json:"driver"`
        Connstr string `json:"connstr"`
        MaxOpenConns int `json:"max_open_conns,omitempty"`
    }{driver, connstr, maxOpen})
}
```

### New packages `providers/<type>/caddyfile`

One per provider, following the same pattern:

- `module.go`: `caddy.RegisterModule`, `CaddyModule()` ID
  `xtemplate.providers.<type>`, compile-time `var _ xtc.CaddyfileProvider =
  (*<type>Caddyfile)(nil)` guard, blank import of `providers/<type>` (bundling).
- `caddyfile.go`: `ParseCaddyfile` curated subset for that provider's JSON
  surface.

### New package `xtemplate/caddy/standard`

- `caddy/standard/standard.go`: blank imports of
  `providers/{sql,fs,flags}/caddyfile`. No code. Imported by caddy builds that
  want the default set's Caddyfile syntax (and, as a side effect, the default
  set's constructors linked). Lives under the `caddy/` tree (so it obviously
  links caddy — no surprise for Go binaries that don't use caddy);
  `xtemplate/caddy` itself stays provider-free, since the subpackage is a
  separate Go package.

### Files to migrate (Caddyfile)

- `caddy/README.md` — replace the JSON-only note left by the registry change
  with a "Provider blocks" section documenting the `provider <type> <field> { }`
  Caddyfile syntax, the curated-subset/JSON escape-hatch relationship, and the
  `--with .../xtemplate/caddy/standard` opt-in for the default set and
  `--with .../providers/<type>/caddyfile` for individual providers.
- `caddy/caddyfile_test.go` — add tests: provider block parse (assert
  `ProvidersRaw` type/name/fields via a small helper struct), error cases
  (missing args, unknown type with derived-path message, unknown block key,
  provider parser emitting a `type`/`name` key → error).
- `providers/<type>/caddyfile/caddyfile_test.go` — new for each provider: unit
  tests for the parser's curated subset (round-trip to the provider's JSON
  shape) and error cases.

### Integration tests (Caddyfile)

`test-caddy` (xcaddy build): add `--with
  github.com/infogulch/xtemplate/caddy/standard=./caddy/standard`
  and `--with
  github.com/infogulch/xtemplate/providers/nats/caddyfile=./providers/nats/caddyfile`
  to the build, and add a Caddyfile-based test config exercising `provider sql
  DB { ... }`, `provider fs FS { ... }`, `provider flags Flags { ... }`, and
  `provider nats Nats { in_process_server { dont_listen true } }`. The shared
  `nats.hurl` suite runs unchanged against the resulting instance.

## Explicitly skipped (add when needed)

- Inline (non-block) provider syntax — block-only for now; add when a provider has a one-line common case.
- Caddyfile coverage of nats jetstream options — Go-API-only (no JSON representation), so no Caddyfile representation either.
- A complete Caddyfile mirror of each provider's JSON surface — curated subset; JSON remains the full-fidelity escape hatch.
