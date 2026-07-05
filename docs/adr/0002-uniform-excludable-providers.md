---
status: accepted
---

# Every dot provider is an excludable package

xtemplate's dot providers live in their own packages at
`xtemplate/providers/<type>` (`dotsql`, `dotfs`, `dotflags`, `dotnats`) and are
linked only when a binary imports one. The `xtemplate` package holds no provider
types, so a binary that never imports a provider never links its dependencies.
We call the packages xtemplate itself ships **core providers**, but they are
ordinary provider packages with no special status.

## Considered options

- **Keep db/fs/flags in the `xtemplate` package, move only nats out.** Their
  dependencies are lightweight (`database/sql` is stdlib; `afero` was already a
  core dependency), so keeping them in-core wouldn't force heavy linking.
  Rejected: it creates a two-class system where some providers are excludable
  and some are not with no principled rule for which class a new provider joins.
  Moving every provider out erases the classification question entirely; the
  cost is a few explicit import lines per binary, which double as a manifest of
  what's linked.

## Consequences

- Breaking Go-API and JSON-config change (pre-1.0, taken as a clean break): the
  four typed config slices collapse into one `providers` array with a `type`
  discriminator, and all provider symbols move to their `providers/<type>`
  packages.
- Go binaries opt in via explicit per-provider imports, though all the default
  binary distributions include all core providers. Caddy users can get the
  standard provider set with the `caddy/standard` package (see ADR-0003).
