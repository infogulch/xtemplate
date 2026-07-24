---
status: accepted
---

# Caddyfile provider dispatch via Caddy’s module registry

Caddyfile `provider <type> <field> { }` and `source <type> { }` are JSON adapters only. Modules under `xtemplate.providers.*` / `xtemplate.source.*` implement `CaddyfileBlockParser` (formerly `CaddyfileProvider`). Dispatch looks up the module by type, asserts the interface, invokes `ParseCaddyfile`, then injects reserved keys (`type`/`name` for providers; `type` for sources) into `ProvidersRaw` / `SourceRaw`. This is separate from the provider/source type registries (ADR-0001, ADR-0007) and doesn't force Caddy into those packages (ADR-0002).

## Considered options

- **`ParseBlock` on `Provider` in the provider package.** Rejected: couples every provider to Caddy.
- **Core `BlockParser` hiding Caddy helpers.** Rejected: Caddyfile concepts leak into `xtemplate`; authors lose real `httpcaddyfile.Helper` APIs.
- **Separate Caddy-side register map.** Rejected: prefer Caddy’s module registry (same pattern as `xtemplate.funcs.*`).

## Consequences

- Only `providers/*/caddyfile` packages import Caddy; `providers/<type>` stays Caddy-free.
- `caddy/standard` blank-imports caddyfile packages for the default set for one-`--with` builds.
- Injected `"type"` / `"name"` must not appear in parser output (parse error).
- Core proviers' Caddyfile config options are a curated subset of JSON; the complete config surface is available via JSON / Go API.
