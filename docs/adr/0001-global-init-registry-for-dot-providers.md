---
status: accepted
---

# Global `init()` registry for dot providers

Dot providers self-register a `type` string → constructor in a package-level map
(`xtemplate/providers.go`) from their package `init()`. During config
resolution, `resolveProviders` peeks the `"type"` discriminator on each JSON
config entry, looks up the constructor, and re-decodes into the concrete type. A
binary opts into a provider simply by importing its package, so the linked
provider set is exactly the import set.

The map is written only from `init()` and read-only afterward. Go runs all
`init()` functions on a single thread before `main`, so every write
happens-before any concurrent reader; no races are possible (unless runtime
registration is ever supported).

## Considered options

- **Explicit, non-global registry** - `Config` holds a `*Registry` and the user
  calls `Register("nats", ...)` in `main`. Rejected: `xcaddy` generates `main`,
  so a Caddy user has no place to call `Register`. The global `init()` registry
  is the only mechanism that fits both the `app.Main` path and the Caddy path,
  and it mirrors how Caddy's own plugin ecosystem assembles.

## Consequences

- The `"type"` JSON key is reserved by the discriminator. A provider struct with
  a `json:"type"` string field is silently overwritten with the type string
  during decode (there is no `DisallowUnknownFields`), so provider configs must
  not reuse that key. (The Caddyfile path enforces this with an explicit error
  but the registry decode does not. See ADR-0003.)
- Provider config errors (unknown `type`, bad settings) surface at instance
  build time inside `resolveProviders`, not at config-load time.
