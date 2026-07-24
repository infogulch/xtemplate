# ADR-0007: Template sources (mirror provider system)

## Status

Accepted (implemented with template-sources plan).

## Context

Reload behavior was forked across adapters (`TemplatesDir` / `TemplatesFS` / `Reload`, `app/watchfs`, `app/git`, Caddy `WatchTemplatePath`). Providers already solve optional linking + JSON + Caddyfile. Sources need the same pattern so CLI and Caddy share implementations without multiple `cmd/*` packages.

## Decision

- One required `TemplateSource` per Server, self-registered via `RegisterSource` (`type` → ctor).
- Config: `Source` + `SourceRaw` + private `templatesFS`. Drop public dir/FS/Reload fields.
- `Start(ctx, log) (initial, reloads, err)` once per Server; nil `initial` means not ready (placeholder MemMapFs with `{{define "ANY /"}}` → 503). Reload options are not sticky; when initial was nil, every Reload must include `WithTemplateFS`/`WithTemplateDir`.
- Built-ins `os` / `fs`; optional `sources/watchfs`, `sources/git`.
- Unified `cmd/xtemplate` + `app.LoadConfig` pass-0 `--source-type`.
- Caddy: `source <type> { }` via `CaddyfileBlockParser`; default `os` (no watch).
- Provider registration renamed `Register` → `RegisterProvider` (no alias).

## Consequences

Breaking for CLI multi-cmd, Caddy default watch, and public Config fields. Legacy JSON/Caddy knobs hard-reject until 1.0. Related: ADR-0001, ADR-0003, ADR-0004.
