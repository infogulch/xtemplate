# xtemplate documentation

xtemplate is a web server that turns a directory of Go templates into a web
application.

See [glossary](reference/glossary.md) for terminology used throughout.

### Tutorials

- [Getting started](tutorial/getting-started.md) — build your first xtemplate app

### How-to guides

- [Custom build](how-to/custom-build.md) — drivers, embed, FuncMaps, providers
- [Create a custom dot provider](how-to/create-a-provider.md)
- [Add authentication with caddy-security](how-to/auth-caddy-security.md)

### Reference

- [Glossary](reference/glossary.md) — bounded context / terminology
- [Deployment modes](reference/deployment-modes.md) — Docker, CLI (plain/watchfs/git), Caddy, library
- [CLI reference](reference/cli.md)
- [Configuration](reference/configuration.md)
- [Instance loading](reference/instance-loading.md)
- [Template semantics](reference/template-semantics.md)
- [Dot context](reference/dot-context.md)
- [Template functions](reference/functions.md)

### Explanation

- [Design](explanation/design.md)
- [Project history](explanation/history.md)

## Development

- [Contributing guide](contributing.md)

### Architecture decision records

Architecturally relevant decisions with rationale and consequences. See
[ADR](https://adr.github.io/).

1. [Global init registry for dot providers](adr/0001-global-init-registry-for-dot-providers.md)
2. [Uniform excludable providers](adr/0002-uniform-excludable-providers.md)
3. [Caddyfile provider dispatch via caddy module registry](adr/0003-caddyfile-provider-dispatch-via-caddy-module-registry.md)
4. [Reload swaps a new immutable instance](adr/0004-reload-swaps-a-new-immutable-instance.md)
5. [Buffered vs flushing handlers](adr/0005-buffered-vs-flushing-handlers.md)
6. [Reflection-assembled dot context](adr/0006-reflection-assembled-dot-context.md)
