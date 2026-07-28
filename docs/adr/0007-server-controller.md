# ADR-0007: Server controllers

## Status

Accepted

## Context

Reload behavior was forked across adapters (`TemplatesDir` / `TemplatesFS` / `Reload`, `app/watchfs`, `app/git`, Caddy `WatchTemplatePath`). Providers already solve optional linking + JSON + Caddyfile. Controllers need the same pattern so CLI and Caddy share implementations without multiple `cmd/*` and `caddy/*` packages.

## Decision

- Optional config-selected **`ServerController`** per Server, self-registered via `RegisterController` (`type` → ctor). JSON/Caddyfile key is `"controller"`.
- Config: `Controller` + `ControllerRaw` + private per-build `TemplateFS`.
- Interface:

  ```go
  type ServerController interface {
      Init(ctx context.Context, log *slog.Logger) (sticky []Option, err error)
      Start(server *Server) error
  }
  ```

  `Init` returns sticky base options. `Start` runs after sticky apply and may call `server.Reload`.
- **Sticky base** from `Init` (or `WithTemplateFS`/`Dir`) is fixed at construction. Ephemeral Reload options apply only to that build.
- **`Server.Reload` is the only adopt path.** Nil instance → HTTP 503.
- Controllers are **Server-only**. Standalone `Instance` requires a resolved template FS.
- Built-in `os`; optional `watchfs`, `git`. In-process FS via `WithTemplateFS`.
- **`DefaultControllerType`** (core `"os"`). Binaries set it (`cmd/xtemplate`, `caddy/standard` → `"watchfs"`). Packages only register types.
- Unified CLI + Caddyfile `controller <type> { }`. Related: ADR-0001, ADR-0003, ADR-0004.

## Consequences

Breaking for channel-based sources and the old public dir/FS/Reload fields. Legacy JSON/Caddy knobs hard-reject until 1.0.

## Rejected designs

- **Single `Start(ctx, log, *Server) (sticky, err)`** that both returned sticky options and started background work. Sticky was applied only after return, under the construction mutex, so controllers could not call `Reload` before returning (deadlock). Documenting a temporal protocol was brittle; splitting Init/Start makes Reload safe in `Start` without special casing.
- **Optional packages reassigning `DefaultControllerType` in `init`** (e.g. blank-import `watchfs` flips the process default). Registration and product default are separate concerns; only composition roots set the default.
- **Placeholder / fake template FS for “not ready”** instead of nil instance + HTTP 503.
- **Channel-based `Config.Reload`** and separate `app/git` / `app/watchfs` entrypoints instead of one controller interface shared by CLI and Caddy.
