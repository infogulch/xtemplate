# Design

xtemplate turns a directory of Go templates into a complete web application. Here's how. Terminology is defined in [glossary](../reference/glossary.md).

## The file path is the route

Files under the template root map to routes. A path template has its extension stripped, so `/admin/settings.html` is the handler for `GET /admin/settings`. Static files are served as-is. No central routing table, no named handler function, just the template root.

Path parameters in the file path become ServeMux wildcards. A path template at `/posts/{slug}.html` handles requests like `GET /posts/my-first-marathon`; the template can read the path value with `{{.Req.PathValue "slug"}}`.

Other HTTP methods use a define template whose name is a ServeMux pattern, e.g. `{{define "METHOD /path/{param}"}}`.

See [Instance loading](../reference/instance-loading.md).

## The template is the handler

Templates are the central primitive and drive the request–response lifecycle. They are not a rendering step inside a separate handler; xtemplate inverts control so the template *is* the handler. There is no custom handler / glue code before or after execution.

### Dot context

The dot context is the sole channel through which templates reach request data, response control, and backing data sources. It is a struct assembled per request from builtin providers (`.X`, `.Req`, and `.Resp` or `.Flush` by handler kind) plus any configured core or custom dot providers. This is what makes templates expressive enough to act as handlers directly.

See [Dot context](../reference/dot-context.md), [ADR 0005 - Buffered vs flushing handlers](../adr/0005-buffered-vs-flushing-handlers.md), and [ADR 0006 - Reflection-assembled dot context](../adr/0006-reflection-assembled-dot-context.md).

### Safe escaping by default

The simplest patterns should be safe by default: templates are parsed and executed as `html/template` with context-aware escaping. The BlueMonday sanitizer and the `trustHtml` / `trustJS` template functions are the deliberate off-ramp when you opt out.

## Static files are first-class

xtemplate is a complete server, so static files get the same care as templates.

They are served from the filesystem with OS-specific optimizations when available (e.g. `sendfile(2)`), minimizing CPU usage so single-origin deployments can use xtemplate as a lightweight CDN stand-in.

Static files are read and hashed when the instance is built, which powers content-addressed URLs and content negotiation. See [Instance loading - static files](../reference/instance-loading.md#static-files).

## Embeddable library

While xtemplate inverts control at the handler level, at the program level it is a library embeddable in any Go program: standalone CLI, Docker, Caddy plugin, or another application.

### Immutable core, reloadable shell

An instance loads the template root once and is immutable thereafter. The server holds the current instance and can reload by building a new instance and swapping it in atomically.

That keeps reloads simple: a running instance is never mutated. The same pattern underpins every deployment mode:

- watchfs CLI (reload on template-root changes)
- git CLI (fetch and reload on remote updates)
- Caddy module (live reconfiguration / optional template watch)

See [Deployment modes](../reference/deployment-modes.md) and [ADR 0004 - Reload swaps a new immutable instance](../adr/0004-reload-swaps-a-new-immutable-instance.md).

---

This design enables app authors to focus on the web's primitives: request paths, templates, and static files.
