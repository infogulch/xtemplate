---
status: accepted
---

# Every dot provider is an excludable package

Core providers live under `providers/` (`dotsql`, `dotfs`, `dotflags`,
`dotnats`) and link only when imported. The `xtemplate` package holds no
provider types or their heavy deps. “Core” means shipped by this repo, not
special runtime status — same shape as any provider package.

## Considered options

- **Keep light providers in-core, only split heavy ones (e.g. nats).** Rejected:
  two-class system with no principled rule for the next provider. Uniform
  packages cost a few blank imports per binary and make the import list the link
  manifest.

## Consequences

- Pre-1.0 break: typed config slices → one `providers` array with a `type`
  discriminator (ADR-0001); symbols move to `providers/…`.
- Default CLI/Caddy builds still blank-import the standard provider set
  (ADR-0003).
