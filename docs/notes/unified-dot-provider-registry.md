# Unified dot-provider registry for excludable providers

Replace xtemplate's per-type config slices with a single registry-based list, mirroring Caddy's plugin-assembly model, so every dot provider — not just heavyweight ones — lives in a provider package at `xtemplate/providers/<type>` and is excludable from binaries that don't use it. A binary that doesn't import a provider package doesn't link its dependencies. The "core provider" concept (db/fs/flags staying in the `xtemplate` package) is eliminated: every provider follows one pattern. Config uniformity and excludability are both consequences of this single mechanism.

This change is **JSON-config only**. Caddyfile `provider <type> <field> { }` syntax is a follow-up: see [caddyfile-provider-blocks.md](./caddyfile-provider-blocks.md). Caddy users continue to configure providers via JSON — the status quo, since the Caddyfile has no provider syntax today.

## Context

xtemplate's core `Config` struct held typed slices (`Databases`, `Directories`, `Flags`, `Nats`) for each dot provider kind, and the dot-provider types themselves (`DotDB`, `DotDir`, `DotFlags`, `DotNats`, `DotKV`) lived in the `xtemplate` package. Because `DotNatsConfig`, `DotNats`, and `DotKV` named nats-server/nats.go/jetstream types directly, the core `xtemplate` package had to import those libraries — so every binary that imported `xtemplate` linked nats-server and nats.go transitively, even binaries that never used nats. The db/fs/flags providers' own dependencies were lightweight (database/sql is stdlib; afero was already a core dep for `TemplatesFS`), and the heavyweight SQL drivers (e.g. ncruces/go-sqlite3) were already excludable via per-binary blank imports — but the provider types themselves rode along in core with no excludability path.

## Mechanism (the Caddy trick, hardened)

One global registry, one centralized decode helper. Provider packages self-register in `init()`. Config carries `ProvidersRaw []json.RawMessage` (JSON config) and `Providers []DotConfig` (Go-API values; renamed from `CustomProviders`, the existing field that `WithProvider` appends to). The helper peeks the `"type"` discriminator, looks up the constructor, and re-decodes into the concrete type. The registry consumes only JSON.

The registry is a package-level `map[string]func() DotConfig` written only in `init()` and read-only afterward; the supported `init()` path is single-threaded before `main`, so the map is race-free by Go's `init()` happens-before guarantee and needs no mutex. The registry code lives in one new file, `xtemplate/providers.go`, holding the map, `Register`, and `resolveProviders`. Duplicate registrations panic with a message naming the type string; the registering package is identified by the stack trace the runtime prints automatically (no `runtime.Caller` introspection in `Register`).

Every dot provider moves to a provider package at `xtemplate/providers/<type>`: `sql`, `fs`, `flags`, `nats`. The `xtemplate` package exports only the registry, the `DotConfig` interface, and `WithProvider` (for custom providers that bypass the registry). A binary opts into a provider by importing its package (blank import for `init()`-only users, or named import for the Go API). The "core provider" concept is eliminated — there is no special class of provider that stays in the `xtemplate` package.

The default-enabled set — the providers xtemplate's own binaries ship with — is `{sql, fs, flags}`. There is no `providers/standard` aggregator package: Go binaries import the provider packages they actually use explicitly (e.g. `_ "github.com/infogulch/xtemplate/providers/sql"`), so the import list is a literal manifest of what's linked. A binary that wants a subset imports individual provider packages. `xtemplate/app` is provider-free; each `cmd/*` binary opts in via explicit per-provider imports (plus its SQL driver). `xtemplate/caddy` is unchanged in code — it embeds `Config` and picks up the new shape automatically; caddy users opt into providers via xcaddy `--with` (see Consequences). Caddyfile syntax for the default set is the follow-up note.

## Considered options

- **Explicit (non-global) registry**: `Config` holds a `*Registry`, user calls `Register("nats", ...)` in `main`. Rejected because it breaks the import-based opt-in that Caddy's ecosystem (and xcaddy) relies on — there's no place for a caddy user to call `Register` since xcaddy generates the main. The global `init()` registry is the only mechanism that fits both the `app.Main` path and the caddy path.
- **Keep nats in core, uniformize config only**: collapses the slices but leaves core importing nats. Defeats the primary excludability goal.
- **Keep db/fs/flags in core, move only nats out**: the original draft's position. db/fs/flags have lightweight deps (database/sql is stdlib; afero is already a core dep), so keeping them in-core doesn't force heavyweight linking. Rejected in favor of uniformity: the "core provider" concept creates a two-class system where some providers are excludable and others aren't, with no principled rule for which class a new provider joins. Moving all providers to `providers/<type>` eliminates the classification question entirely — every provider follows one pattern — and the cost is low (explicit per-provider imports are a few lines per binary and double as a manifest of what's linked).
- **A `caddy/standard` aggregator for JSON-only caddy users**: blank-import the provider packages (no Caddyfile) so a caddy JSON user gets the standard set linked in one `--with`. Rejected for now on uniformity grounds — caddy JSON users follow the same explicit-import rule as Go binaries (`--with .../providers/<type>` per provider). Reconsider when the Caddyfile follow-up lands: at that point `xtemplate/caddy/standard` blank-imports `providers/{sql,fs,flags}/caddyfile`, which transitively blank-imports `providers/{sql,fs,flags}`, so it becomes the one-opt-in aggregator for the standard set for both Caddyfile and JSON caddy users.

## Consequences

- **JSON config shape breaks** (pre-1.0, clean break): `"databases"`/`"directories"`/`"flags"`/`"nats"` keys collapse into one `"providers"` array with a `"type"` discriminator on each entry. The `"databases"` type renames to `"sql"`.
- **Go API breaks for all provider symbols**: `WithDB`/`DotDB`/`DotDBConfig`, `WithDir`/`Dir`/`DotDirConfig`, `WithFlags`/`DotFlags`/`DotFlagsConfig`, `WithNats`/`NatsConfig`/`DotNatsConfig`/`DotNats` move from `package xtemplate` to their provider packages (`providers/sql`, `providers/fs`, `providers/flags`, `providers/nats`). Symbols are renamed to match the type string: `WithSql`/`DotSql`/`DotSqlConfig`, `WithFs`/`DotFs`/`DotFsConfig`. `WithFlags`/`DotFlags`/`DotFlagsConfig` and `WithNats`/`DotNats`/`DotNatsConfig` already match and keep their names. The `With*` functions move alongside their config types, append to `Config.Providers` (the renamed `CustomProviders`), and return `xtemplate.Option` — so each provider package imports `xtemplate` (one-directional: provider → core, never the reverse). Migration is an import path swap plus a symbol rename for sql/fs. Templates are unaffected (they access providers via reflection on field names, never naming the Go package).
- **`DotKV` is deleted**: it was dead code (nothing constructed it, it didn't implement `DotConfig`, no test or template referenced it) and it imported `nats.go/jetstream` directly — so deleting it is load-bearing for the excludability goal: without deletion, the `xtemplate` package would still import `nats.go/jetstream` even after nats moves to `providers/nats`. The only references today are a README link and a stale `"DotKVProvider"` error string in `dot_flags.go`'s `WithFlags` (a copy-paste leftover); both go. A future nats-backed KV provider would live in or beside `providers/nats`, not as a generic `providers/kv`.
- **`DotDirConfig.Type` is deleted**: it was a speculative backend-selection knob (`json:"type"`) never wired up (`Init` hardcodes `afero.NewBasePathFs(afero.NewOsFs(), p.Path)`). The `"type"` JSON key is reserved by the registry discriminator; any provider struct field keyed `json:"type"` would collide (the discriminator decodes into it, silently setting it to the provider-type string). The rule "the `type` key is reserved by the registry" is a documented constraint on all future provider structs. A future fs-backend knob should use a non-colliding key (e.g. `"backend"`).
- **`DotNatsConfig.Conn` gets `json:"-"`**: today it's exported with no tag, so it sits in the JSON surface as `"Conn"`. It's a runtime `*nats.Conn` (a struct of mutexes, channels, net.Conn, and unexported runtime state); JSON-decoding into it produces a non-nil but hollow shell that `Init` accepts and that panics on first use. The `json:"-"` tag prevents the JSON path from ever populating it; the Go-API path (`WithNats`/`WithProvider(&DotNatsConfig{Conn: nc})`) is unaffected. `Init`'s existing `Conn != nil` early-return branch (the Go-API/test path) stays as-is.
- **Nats jetstream options are Go-API-only**: `NatsConfig.JetStreamOptions` is `[]jetstream.JetStreamOpt` — a slice of `func(*jetstream.Options)`, which has no JSON representation. `json.Marshal` on a non-nil function slice returns `unsupported type: func`; `json.Unmarshal` silently leaves it nil. The curated-subset/JSON-escape-hatch framing does not cover jetstream opts: they are settable only via the Go API (`WithNats(..., jsOpts)`), and a caddy user who needs them must use a custom build with `WithNats`.
- **`xtemplate/app` is provider-free**: `app.Main` users who want the standard set add explicit blank imports per provider (`_ "github.com/infogulch/xtemplate/providers/sql"`, etc.); those who want nats add `_ "github.com/infogulch/xtemplate/providers/nats"`; those who want a subset import individual provider packages.
- **`xtemplate/caddy` is unchanged in code**: it embeds `xtemplate.Config` and so picks up the new `ProvidersRaw`/`Providers` shape automatically; no provider types are referenced in the caddy package. Its JSON test config (`caddy/module_test.go`, `test/caddy.json`) and the `caddy/README.md` parenthetical update to the `"providers"` array. Caddyfile provider syntax is the follow-up.
- **A caddy binary built with only `--with .../xtemplate/caddy` links NO dot providers**: the caddy package imports no provider packages, so none register. A caddy JSON user must add `--with github.com/infogulch/xtemplate/providers/<type>` for each provider type their config references — the same explicit-import rule as Go binaries. An unknown `"type"` in a caddy JSON config fails at registry lookup inside `Instance()` with the standard "unknown provider type" error (see Registry error timing below). This is a build-time break for caddy JSON users on top of the config-shape break; it is the excludability win reaching the caddy path.
- **Registry error timing shifts**: provider config errors surface at Instance build time (when `resolveProviders` decodes `ProvidersRaw`), not at config load time. `ProvidersRaw` is `[]json.RawMessage`, so `app`'s `LoadConfig` captures each entry as raw bytes without interpreting it; a typo'd `"type"` discriminator fails at registry lookup inside `Instance()`, and a typo'd setting key is silently dropped (standard `encoding/json` behavior). This matches the `CONTEXT.md` "Provider config decoding" entry.
- **Provider package names collide with stdlib**: `providers/sql` (package `sql`) collides with `database/sql`; `providers/fs` (package `fs`) collides with `io/fs`; `providers/nats` (package `nats`) collides with `github.com/nats-io/nats.go`. Go-API users who import both must alias one. **Convention**: the provider package keeps its bare name; the caller aliases the stdlib or external library (e.g. `stdsql "database/sql"`, `natsgo "github.com/nats-io/nats.go"`). Document this convention in `CONTEXT.md`. Accepted as a uniform cost (JSON-only/blank-import users never name the package), and consistent with the rule that package name = type string for all providers. Provider source files themselves need no aliasing — their own package name is never in scope.

## Implementation

### New file `xtemplate/providers.go`

The registry map, `Register(name string, ctor func() DotConfig)`, and `resolveProviders(raw []json.RawMessage) ([]DotConfig, error)`. The map is `init()`-write-only; a plain-language comment names the ceiling and upgrade path ("init()-only writes; the map is race-free by Go's init happens-before guarantee. Add a sync.RWMutex if runtime registration becomes supported."). `resolveProviders` peeks `"type"` from each raw message, looks up the constructor, re-decodes into the concrete type. Unknown-type error: for `typeName` in `{sql, fs, flags, nats}`, derive the import path (`add it by importing github.com/infogulch/xtemplate/providers/<typeName>`); otherwise, generic (`unknown provider type %q; ensure the provider package that registers type %q is imported`).

### New file `xtemplate/providers_test.go`

The registry is the one new mechanism with no existing test coverage. Add a `providers_test.go` in `package xtemplate` that registers a test-only provider in `init()` and exercises `resolveProviders` directly: round-trip a known-good config, verify the unknown-type error path (otherwise unguarded), and verify duplicate registration panics with the expected message. Keeping tests in `package xtemplate` lets them call `resolveProviders` directly and reuse shared helpers (`buildInstance`, `doRequest`, `newMemFS`) without re-exporting internals; the `With*` functions they exercise are now the moved versions in each provider package.

### `xtemplate/config.go`

Replace `Databases`/`Directories`/`Flags`/`Nats` typed slices with `ProvidersRaw []json.RawMessage`; rename `CustomProviders` → `Providers`. `WithProvider` appends to `Providers` (unchanged behavior, renamed field). `SetDefaults` needs no change (no provider defaults).

### `xtemplate/instance.go`

Replace the four typed-slice loops (lines 161-172) with `dot = resolveProviders(build.config.ProvidersRaw)` then `dot = append(dot, build.config.Providers...)`. The dedup + `Init` loop is unchanged.

### Provider moves

- `xtemplate/dot_kv.go` — delete (dead code, load-bearing for excludability).
- `xtemplate/dot_db.go` + `dot_db_config.go` → `providers/sql/` (renamed `DotSql`/`DotSqlConfig`/`WithSql`).
- `xtemplate/dot_fs.go` + `dot_fs_config.go` → `providers/fs/` (renamed `DotFs`/`DotFsConfig`/`WithFs`; `Dir` type renamed `DotFs` for full `Dot<Type>` uniformity — only a handful of refs in 2 source files; templates are unaffected since they access the provider by field name only); declare a local `var bufPool sync.Pool` in `providers/fs` (4 lines of stdlib — do not export `xtemplate`'s internal pool; one-directional dep graph wins over exporting an internal detail); **delete `DotDirConfig.Type`** (speculative `json:"type"` knob never wired up; the `"type"` key is reserved by the registry discriminator).
- `xtemplate/dot_flags.go` → `providers/flags/` (names unchanged); fix the stale `"DotKVProvider"` error string in `WithFlags` → `"DotFlagsProvider"`.
- `xtemplate/dot_nats.go` + `dot_nats_config.go` → `providers/nats/` (names unchanged); add `json:"-"` to `DotNatsConfig.Conn`.

### `cmd/main.go` + `cmd/git/main.go` + `cmd/watchfs/main.go`

Add explicit blank imports of all four providers — `providers/sql`, `providers/fs`, `providers/flags`, `providers/nats` — alongside the existing SQL driver import. (`nats` is included because the `nats.hurl` integration suite covers the nats path and dropping it would lose that coverage for no architectural gain; the excludability win applies to other binaries that don't import it.) (The original draft listed only `cmd/main.go` + `cmd/git/main.go`; `cmd/watchfs/main.go` follows the same pattern and needs the same imports.)

### JSON config rewrites — `test/config.json` + example configs

Exactly five JSON configs carry provider keys and need rewriting to the `"providers"` array; `test/caddy.json` is handled in the caddy section above. The remaining four: `test/config.json` (which has a nats entry and must keep it — dropping it would break the `nats.hurl` integration coverage) and the three example JSON configs. All follow the same shape: replace the `"databases"`/`"directories"`/`"flags"`/`"nats"` top-level keys with a `"providers"` array of objects each carrying a `"type"` discriminator. The `sse-chat`, `hub`, and `dotprovider` example configs set only `"templates_dir"` and need no changes.

### `caddy/README.md` + `caddy/module_test.go` + `test/caddy.json` + the xcaddy build

- `caddy/README.md` — update the parenthetical "(databases, directories, flags, nats)" to "(providers)"; keep the JSON-only note (Caddyfile provider syntax is the follow-up). The `[caddy.json]` reference stays.
- `caddy/module_test.go` — rewrite the JSON config (`"databases"`/`"directories"`/`"flags"` keys) and the field assertions to the `"providers"` array with `"type"` discriminators. The test only `json.Unmarshal`s into the module struct and never calls `Instance()`, so `ProvidersRaw` stays as raw bytes — no provider package import is needed for this file.
- `test/caddy.json` — rewrite the four typed arrays as one `"providers"` array (`{"type":"sql","name":"DB",...}`, `{"type":"fs","name":"FS","path":"data"}`, `{"type":"fs","name":"FSW","path":"."}`, `{"type":"flags","name":"Flags","values":{...}}`, `{"type":"nats","name":"Nats","nats_config":{"in_process_server_options":{"dont_listen":true}}}`).
- The xcaddy build for the caddy test binary (`xt-build-caddy` in `.config/mise/lib.nu` / the `build-caddy` task) adds `--with github.com/infogulch/xtemplate/providers/sql`, `.../providers/fs`, `.../providers/flags`, `.../providers/nats` (the test config uses all four).
- The shared hurl suite (`test/tests`) runs unchanged against the resulting instance.

### `README.md`

Delete the `DotKV` line + its link definition; update the `[Dir]`/`[DotDB]` links to `providers/fs#DotFs` and `providers/sql#DotSql` (and fix the broken `[DotFS]` label to `[DotFs]`); update the "built-in `DotDB`" reference to the `sql` (`DotSql`) provider.

### `app/git/`, `app/watchfs/`, `examples/dotprovider/main.go`

No changes. `app/git/` and `app/watchfs/` are provider-free (verified against source: neither file imports or names any provider type). `examples/dotprovider/main.go` uses only `WithProvider` (the Go-API bypass, unaffected by the registry move; verified).

### `CONTEXT.md`

Add one entry documenting the aliasing convention for provider package name collisions: provider keeps bare name, caller aliases stdlib/external library (`stdsql "database/sql"`, `natsgo "github.com/nats-io/nats.go"`). No other rewrites needed — `CONTEXT.md` already uses the target terminology (`DotConfig`, `providers/<type>`, etc.).

### `CHANGELOG.md`

Add an `[Unreleased]` section (matching the v0.9.2 format already in the file) with:
- `**Breaking:**` — JSON config shape: `"databases"`/`"directories"`/`"flags"`/`"nats"` keys collapse into a `"providers"` array with `"type"` discriminators; `"databases"` type string renamed to `"sql"`.
- `**Breaking:**` — Go API: all provider symbols (`WithDB`/`DotDB`/`DotDBConfig`, `WithDir`/`Dir`/`DotDirConfig`, `WithFlags`/`DotFlags`, `WithNats`/`DotNats`/`DotNatsConfig`) move from `package xtemplate` to their respective `providers/<type>` packages; `sql`/`fs` symbols renamed.
- `**Removed:**` — `DotKV` (dead code; load-bearing for excludability). `DotDirConfig.Type` (speculative, never wired up).

## Implementation log

### Bite 1 — `xtemplate/providers.go` + `xtemplate/providers_test.go` ✓

- Created `xtemplate/providers.go`: package-level `registry` map, `Register(name, ctor)` (panics on duplicate), `resolveProviders([]json.RawMessage)` (peeks `"type"`, looks up ctor, re-decodes; well-known-type hint in error for `sql`/`fs`/`flags`/`nats`).
- Created `xtemplate/providers_test.go` (`package xtemplate`): test-only `_test` provider registered in `init()`; four tests — round-trip decode, well-known unknown type, generic unknown type, duplicate-registration panic. All pass.
- No existing files touched.

### Bite 2 — `xtemplate/config.go` + `xtemplate/instance.go` + four `With*` files + `caddy/module_test.go` ✓

- `config.go`: replaced `Databases`/`Flags`/`Directories`/`Nats` typed slices + `CustomProviders` with `ProvidersRaw []json.RawMessage` (JSON) and `Providers []DotConfig` (Go API); `WithProvider` now appends to `Providers`.
- `instance.go`: replaced the four typed-slice loops with `resolveProviders(build.config.ProvidersRaw)` + `append(dot, build.config.Providers...)`.
- `dot_db_config.go`, `dot_fs_config.go`, `dot_flags.go`, `dot_nats_config.go`: each `With*` function now appends a pointer to `c.Providers` instead of the old typed slice. Fixed stale `"DotKVProvider"` error string in `WithFlags` → `"DotFlagsProvider"`. Added `json:"-"` to `DotNatsConfig.Conn`.
- `caddy/module_test.go`: updated JSON fixture to `"providers"` array with `"type"` discriminators; replaced typed-slice assertions with `len(m.ProvidersRaw) == 3`.
- Build and targeted tests pass.

### Bite 3 — Provider moves + test relocation ✓

- **Deleted** `dot_db.go`, `dot_db_config.go`, `dot_fs.go`, `dot_fs_config.go`, `dot_flags.go`, `dot_nats.go`, `dot_nats_config.go`, `dot_kv.go` from the `xtemplate` package.
- **Created `providers/sql/sql.go`**: `DotSqlConfig`/`DotSql`/`WithSql` (renamed from `DotDBConfig`/`DotDB`/`WithDB`); `init()` registers `"sql"`.
- **Created `providers/fs/fs.go`**: `DotFsConfig`/`DotFs`/`WithFs` (renamed from `DotDirConfig`/`Dir`/`WithDir`); deleted `DotFsConfig.Type` field; local `bufPool`; `init()` registers `"fs"`.
- **Created `providers/flags/flags.go`**: `DotFlagsConfig`/`DotFlags`/`WithFlags` (names unchanged); `init()` registers `"flags"`.
- **Created `providers/nats/nats.go`**: `DotNatsConfig`/`NatsConfig`/`DotNats`/`WithNats` (names unchanged); `json:"-"` on `Conn` retained; `init()` registers `"nats"`.
- **Test relocation**: `dot_db_test.go` → `providers/sql/sql_test.go` (package `sql_test`); `dot_nats_test.go` → `providers/nats/nats_test.go` (package `nats_test`); `TestFlagsProvider` → `providers/flags/flags_test.go`; `TestDirProvider` → `providers/fs/fs_test.go`. Each provider test package provides its own `buildInstance`/`doRequest` helpers (public API only).
- **`readme_test.go`**: changed from `package xtemplate` to `package xtemplate_test`; inline helpers added; imports `providers/sql` and `providers/fs`; all `WithDB`/`WithDir` calls replaced with `provsql.WithSql`/`provfs.WithFs`.
- **`instance_test.go`**: removed `TestServeHTTP_FlagsProvider` and `TestServeHTTP_DirProvider` (moved to provider packages).
- **`providers_test.go`**: updated `TestResolveProviders_unknownWellKnown` to use `"nats"` (guaranteed unregistered in the xtemplate test binary).
- All packages build and all tests pass.

### Bite 4 — `cmd/` blank imports + JSON config rewrites ✓

- `cmd/main.go`, `cmd/git/main.go`, `cmd/watchfs/main.go`: added blank imports for all four providers (`providers/flags`, `providers/fs`, `providers/nats`, `providers/sql`) alongside the existing sqlite driver import.
- `test/config.json`: replaced `"directories"`/`"databases"`/`"flags"`/`"nats"` top-level keys with a `"providers"` array with `"type"` discriminators on each entry.
- `examples/contacts/config.json`, `examples/blog/config.json`, `examples/filebrowser/config.json`: same shape conversion.
- Build passes.

### Bite 5 — `test/caddy.json` + `caddy/README.md` + xcaddy build task ✓

- `test/caddy.json`: replaced four typed arrays with a single `"providers"` array; each entry carries a `"type"` discriminator.
- `caddy/README.md`: updated the NOTE callout to say "dot providers" instead of the four names; added a note that each provider needs a `--with` flag in the xcaddy build; updated the build examples to show per-provider `--with` flags.
- `.config/mise/lib.nu` (`xt-build-caddy`): added `--with` for all four provider packages with local path mappings alongside the existing caddy/xtemplate entries.
- Build passes.

### Bite 6 — `README.md` + `CONTEXT.md` + `CHANGELOG.md` ✓

- `README.md`: replaced broken `[DotFS]`/`[DotDB]`/`[DotKV]` optional-field list with `[DotFs]`/`[DotSql]` pointing to their new provider packages; deleted `DotKV` bullet and link definition; updated `DotDB` → `DotSql` in the custom-provider development note.
- `CONTEXT.md`: added **Provider package name aliasing** entry documenting the aliasing convention (`stdsql`, `natsgo`, etc.).
- `CHANGELOG.md`: appended three items to `[Unreleased]`: JSON config shape break, Go API symbol moves, and `DotKV`/`DotDirConfig.Type` removals.
- Build passes.

## Explicitly skipped (add when needed)

- Backward-compat aliasing of old JSON keys — pre-1.0, clean break.
- A separate `go.mod` for nats — same-module is enough to stop linking it.
- A nats-backed KV provider (replacing the deleted `DotKV`) — tracked in a follow-up GitHub issue. Would live in or beside `providers/nats`, not as a generic `providers/kv`, since the deleted code was nats-specific.
- Per-provider typed CLI flags — rejected on excludability grounds (the flag parser lives in `app`; for it to know provider-specific flag mappings, `app` must import the provider types, re-creating the coupling the registry removes).
- A `providers/standard` aggregator package for Go binaries — rejected in favor of explicit per-provider imports, which double as a manifest of what's linked. The "standard provider set" survives as a *name* for `{sql, fs, flags}` (documented in `CONTEXT.md`), not as a package.
- Caddyfile provider blocks — follow-up: [caddyfile-provider-blocks.md](./caddyfile-provider-blocks.md).
- A `caddy/standard` aggregator for JSON-only caddy users — reconsider when the Caddyfile follow-up lands (it then aggregates both the Caddyfile parsers and the constructors behind one opt-in).
- JSON-configurable nats jetstream options — `[]jetstream.JetStreamOpt` is function-typed with no JSON representation. A serializable intermediate (e.g. a `JetStreamConfig` struct converted to opts at `Init`) is a real feature but belongs in a separate follow-up, not the registry unification.
