---
status: accepted
---

# Dot context is a reflection-built struct of provider fields

The per-request dot context is not a fixed Go type. When an instance is built,
`makeDot` walks the configured dot providers, calls each `Value` once with a
mock request to learn the concrete Go type of its dot field, and
`reflect.StructOf` builds a struct type whose exported fields are those names
and types. Per request, a pooled value of that type is filled by calling `Value`
again with the real request; optional `CleanupDotProvider` runs after execution
(success or failure).

This is how an open-ended set of core, custom, and registered provider packages
can contribute fields like `.DB` or `.Shop` without the `xtemplate` package
knowing those types at compile time, and without forcing templates onto a
map-based or `interface{}`-only `.`.

## Considered options

- **Fixed context struct in xtemplate** (hard-coded fields). Rejected: every new
  provider would require a core API change, and user custom providers could not
  add fields without forking.
- **Map or `map[string]any` as `.`.** Rejected: loses method sets and typed
  field access that `html/template` and authors rely on (e.g. `.DB.QueryRows`,
  `.Shop.Product`); errors become stringly-typed key mistakes, and accidentally
  mutating the map would corrupt the dot context and cause confusing errors.
- **Code generation of a context type per binary.** Rejected: extra build step
  for every new configuration; the reflection approach keeps "import provider +
  configure" as the only assembly step (see ADR-0001 / 0002).
- **Infer types only from `FieldName` + generics.** Rejected: Go generics would
  not allow customization of the user-chosen field names or multiple fields from
  the same provider.

## Consequences

- `Value` must return a stable, non-nil concrete type. The load-time mock call
  discards the value and uses only `reflect.TypeOf`; a nil interface (including
  `return nil, err` during inference) cannot define a field type and fails
  instance build. Providers that need request data must still return a typed
  zero value of the right type when the mock request is inadequate.
- Field names come from `FieldName()` and must be unique on an instance;
  collisions fail the build.
- Method sets visible to templates are those available on the *value* returned
  by `Value`, not of the provider config type itself.
- Cleanup order and partial unwind on construction failure are part of the
  contract: providers that open resources in `Value` should implement
  `CleanupDotProvider` (as the SQL core provider does for transactions).
- Two dots are assembled per instance (buffered vs flushing provider lists); see
  ADR-0005. The reflection mechanism is shared; the field lists differ by
  `.Resp` vs `.Flush`.
