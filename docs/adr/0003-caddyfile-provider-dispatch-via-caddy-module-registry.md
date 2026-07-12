---
status: accepted
---

# Caddyfile provider dispatch via Caddy’s module registry

Caddyfile `provider <type> <field> { }` is a JSON adapter only. If a provider
registers a Caddy module under the `xtemplate.providers.*` namespace, dispatch
looks up the module by type, type-asserts it to `CaddyfileProvider`, and invokes
it to get the provider's raw JSON config. It then injects `"type"` and `"name"`
and appends to `ProvidersRaw`. This is separate from the provider type registry
(ADR-0001) and doesn't force Caddy into provider packages (ADR-0002).

## Considered options

- **`ParseBlock` on `DotConfig` in the provider package.** Rejected: couples
  every provider to Caddy.
- **Core `BlockParser` hiding Caddy helpers.** Rejected: Caddyfile concepts leak
  into `xtemplate`; authors lose real `httpcaddyfile.Helper` APIs.
- **Separate Caddy-side register map.** Rejected: prefer Caddy’s module registry
  (same pattern as `xtemplate.funcs.*`).

## Consequences

- Only `providers/*/caddyfile` packages import Caddy; `providers/<type>` stays
  Caddy-free.
- `caddy/standard` blank-imports caddyfile packages for the default set for
  one-`--with` builds.
- Injected `"type"` / `"name"` must not appear in parser output (parse error).
- Core proviers' Caddyfile config options are a curated subset of JSON; the
  complete config surface is available via JSON / Go API.
