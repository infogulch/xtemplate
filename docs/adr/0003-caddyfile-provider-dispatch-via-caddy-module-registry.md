---
status: accepted
---

# Caddyfile provider dispatch via Caddy's module registry

Caddyfile `provider <type> <field> { }` blocks are dispatched through Caddy's
own module registry: each provider contributes a Caddy module in the
`xtemplate.providers.*` namespace (in an opt-in `providers/<type>/caddyfile`
subpackage) implementing `CaddyfileProvider` (`ParseCaddyfile(h)
(json.RawMessage, error)`). The `xtemplate/caddy` dispatch looks the module up
by type, calls it, injects the reserved `"type"` and `"name"` keys, and appends
the result to `ProvidersRaw`. This is a pure Caddyfile→JSON adapter and never
touches the dot-provider registry (ADR-0001) or the provider packages, which
stay Caddy-free.

## Considered options

- **Interface assertion on the config type** — look the provider up in the
  dot-provider registry and assert its `DotConfig` has a `ParseBlock` method.
  Rejected: that method would live in the provider package, forcing every
  provider to import Caddy — re-creating the exact coupling ADR-0002 removed.
- **Core-owned `BlockParser` abstraction** — hide `httpcaddyfile.Helper` behind
  an `xtemplate`-owned interface. Rejected: leaks a Caddyfile-shaped concept
  into a package that never reads a Caddyfile, and loses `h.ArgErr()` /
  `h.Errf()` / nested-block support for provider authors.
- **A custom Caddy-side registry** — a separate guarded map with
  `RegisterCaddyfileProvider`. Rejected in favor of reusing Caddy's module
  registry, which `xtemplate.funcs.*` already establishes as the repo's dispatch
  pattern.

## Consequences

- `providers/<type>` stays Caddy-free; only the opt-in
  `providers/<type>/caddyfile` package imports both Caddy and its provider.
- `xtemplate/caddy/standard` blank-imports the default set's `caddyfile`
  subpackages (and thereby their constructors), giving Caddy builds a one-`--with`
  opt-in for `{sql, fs, flags, nats}`.
- The `"type"` and `"name"` keys are reserved for the dispatch to inject; a
  `ParseCaddyfile` implementation whose returned JSON contains either key is
  rejected with a parse-time error, rather than being silently overwritten.
- Caddyfile syntax is a curated subset of each provider's JSON surface; JSON is
  the full-fidelity escape hatch, and options with no JSON representation (e.g.
  nats jetstream `func`-typed options) remain Go-API-only.
