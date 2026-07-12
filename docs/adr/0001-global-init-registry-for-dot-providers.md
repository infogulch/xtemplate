---
status: accepted
---

# Global `init()` registry for dot providers

Provider packages self-register a provider type string → constructor in a package-level map (`providers.go`) from `init()`. Config resolution peeks each JSON entry’s `"type"`, constructs, and re-decodes. A binary’s linked set is exactly the packages it imports. The map is write-once at init (Go’s happens-before), then read-only.

## Considered options

- **Explicit registry on `Config`, register in `main`.** Rejected: `xcaddy` generates `main`, so Caddy builds have nowhere to call `Register`. A global `init()` registry serves both CLI and Caddy (same idea as Caddy’s own plugins).

## Consequences

- `"type"` is reserved on provider config JSON; don’t put a user field there (Caddyfile path errors; registry decode overwrites - see ADR-0003).
- Unknown type / bad settings fail at instance build (`resolveProviders`), not at earlier CLI/JSON parse alone.
