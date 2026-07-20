# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `bus` provider (`providers/dotbus`): process-local multi-producer
  multi-consumer topic fan-out for single-process SSE / live UI.
  Template API: `Publish(topic, message)`, `Subscribe(topic)`. Optional
  `buffer` (default 16); slow subscribers drop rather than block publishers.
  Caddyfile: `provider bus <field> { buffer <n> }`. Included in standard CLI
  and `caddy/standard` builds.
- `fs` provider: optional `writable` flag (default false). When true, the
  template field is `DotFsRW` with `ReceiveFiles` for streaming multipart
  uploads onto the provider FS; when false, the field stays read-only
  (`DotFs`) and the backing FS is wrapped with afero `ReadOnlyFs`.
- `dotfs.WithFsWritable` for Go API opt-in; Caddyfile `writable true` in
  `provider fs` blocks. Init probes writability when `writable` is true.

## [v0.10.0] - 2026-07-12

Uniform provider registry and Caddyfile provider dispatch.

Replace provider slices with a uniform provider registry:

- Breaking: JSON/struct provider configs `"databases"`, `"directories"`,
  `"flags"`, and `"nats"` are removed. These dot providers are now configured by
  adding to the `providers` slice.
- Breaking: Core providers are moved out of `xtemplate` into separate packages
  in `xtemplate/providers`.
- Breaking: `DotDB` and `DotDir` providers are renamed to `DotSQL` and `DotFS`.
- Providers are registered by calling `xtemplate.Register` with their provider
  type name and a constructor function.
- While loading JSON config, the provider is looked up by matching the `"type"` field
  against registered providers and calling the constructor function.

Allow external providers to be configured via Caddyfile:

- Breaking: the default xtemplate caddy module no longer includes the default
  providers. To include them, compile the
  `github.com/infogulch/xtemplate/caddy/standard` module.
- Caddyfile providers register as a `caddy.Module` in the
  `xtemplate.providers.*` namespace with a type that implements the
  `xtemplate/caddy.CaddyfileProvider` interface.
- All core providers can be configured via Caddyfile.

Other changes:

- Removed `DotKV` as dead code. May be re-added in a future release.
- Make `Server` implement `http.Handler` directly
- Fix `CrossOrigin.Disabled` leaving `Instance.handler` nil
- Rebuild app logger after `LoadConfig` so `--loglevel` applies
- Add Caddyfile `precompress` directive
- Link pure-Go sqlite3 driver in `caddy/standard`
- Fix unknown provider import path hint
- Clarify `Config.Handlers` as peer ServeMux routes
- Clean up package names
- Add bounded context glossary, ADRs, and Diátaxis user documentation

## [v0.9.6] - 2026-07-03

- Drop withArgs, use list instead
- Convert hub examples list from js to a Go template equivalent
- Add a nushell-based SSE test harness

## [v0.9.5] - 2026-06-23

Pre-compress static files.

- Follow name change of `upload-zip-artifacts` gh action
- Add Precompress config option to pre-compress static files

## [v0.9.4] - 2026-06-22

CI Polish

- Bump remaining GitHub action versions, fixes build warnings
- Upload dist zips via `infogulch/upload-artifacts@v1`
- Dist zips are versioned with git describe when running ci on a non-tagged commit

## [v0.9.3] - 2026-06-22

- Bump GitHub action versions, fixes release ci workflow

## [v0.9.2] - 2026-06-22

Improvements to file serving, template scanning, server lifecycle, and markdown parsing.

- Serve static files via `sendfile(2)` by unwrapping afero files to the underlying `*os.File`; fixes #78
- Skip hidden directories when scanning templates
- Cancel `.Serve()` via `Config.Ctx` for clean shutdown
- Parse front matter with goldmark-frontmatter; fixes #93
  - **Breaking:** `markdown` now parses front matter and renders the body in one pass, accepting `string`/`[]byte`/`io.Reader` and returning `{.Meta, .Body}`; render plain markdown with `(markdown $s).Body`
  - **Breaking:** removed `splitFrontMatter`; use `markdown`'s `.Meta`/`.Body` instead
  - **Breaking:** only YAML (`---`) and TOML (`+++`) front matter are supported now; JSON and the YAML `...` close fence are dropped
- Cache blog post metadata in an `INIT`-built SQLite table in the blog example
- Set default HTTP server timeouts

## [v0.9.0] - 2026-06-19

Reload-driven apps, a cgo-free SQLite/afero stack, and a major test/tooling overhaul.

- Make config loading reusable and add reload-driven apps `watchfs` & `git`; fixes #76
- Split `app.Main` into reusable `LoadConfig` and `Serve`
- Add `Config.Reload` channel to trigger server reloads
- Add `Config.Handlers` to register custom handlers for routes
- Add example apps demonstrating xtemplate features
- Add `--templates-dir` as an alias for `--template-dir`
- Add `withArgs` to pass extra args to template invocations
- Add `FS.ExistsDir`
- Stream DB query rows with a Go iterator
- Switch DB driver to ncruces/go-sqlite3, dropping cgo
- Migrate the filesystem abstraction to afero
- Add Go 1.25 CORS protection
- Switch to UUIDv7 for request ids
- Log the listening address on server start and the ServeMux pattern matched
- Rename `Config.Defaults` to `SetDefaults`
- Add `AddMarkdownConfig`, deprecating the misspelled `AddMarkdownConifg`
- Default `Minify` to true and make `Config.Minify` a `*bool` so the default holds on all paths
- Make static asset encoding negotiation RFC 7231 compliant
- Fix directory index routes to serve the canonical trailing-slash URL
- Return 503 instead of panicking after `Server.Stop`
- Avoid double `WriteHeader` after `ServeContent`
- Harden `makeDot` against nil provider values; unwind partial dot values on error
- Fall back to build info for version when LDFLAGS are unset
- Remove unused NATS server/client from the Instance struct and other dead code
- caddy: implement `FuncsModules`, add minify and CORS Caddyfile directives, align `templates_dir`
- Add extensive Go, SSE, DB, NATS, caddy, and config tests; guard README examples with tests
- Enable golangci-lint (errcheck, staticcheck) and fix flagged issues
- Replace CUE build tooling with mise; migrate mise tasks to Nushell
- Bump dependencies and Go toolchain to 1.25 (matching Caddy)
- Delete TODO.md; remaining items tracked in issues

## [v0.8.4] - 2025-08-20

Documentation and test polish.

- Recover release notes for 0.6–0.8 and clean up TODOs
- Mention template embedding in features list; fixes #50
- Update readme repository layout section for caddy module changes
- Add go-arg help epilogue examples to match the readme
- Add link tag template test
- Relax match rule
- gofmt caddy package

## [v0.8.3] - 2025-06-17

Move the caddy module back into the main repo and overhaul CI/VSCode tooling.

- Move caddy module to main repo; fixes #47
- Add hello world example
- Remove migration scripts
- VSCode: listen on 8080 in launch.json, use CGO, add "run hurl" task; fixes #44
- Add debug launch profiles for xtemplate and caddy; VSCode setup to debug the test folder
- CI: fix version command, simplify testing to not require cue, allow docker push to fail, skip login when secrets are absent
- Fix static file hash example; fixes #31
- Update README CLI flags listing; fixes #30

## [v0.8.2] - 2025-01-06

Build/release fixes and dependency updates.

- Add logging to caddy tester; always upload logs in GH CI
- Build before zipping; fixes #29; add flag to build caddy with debug symbols
- Update dependencies

## [v0.8.1] - 2024-12-28

- Update dependencies

## [v0.8.0] - 2024-12-26

Simplify the dot provider configuration system.

- Refactor to simplify dot config
- Push docker tags in a loop
- Add lazy-lob-web to the list of users

## [v0.7.1] - 2024-10-09

- Fix docker push syntax
- Wrap db query error message; see #25

## [v0.7.0] - 2024-09-29

Template invocation fixes, new DotFS/try capabilities, and an LDFLAGS-driven build.

- Fix invoking INIT templates with proper dot values
- Change Instance.ServeHTTP so PathValue is set on the request context
- Enhance `try` to be able to call methods
- Add Dir method to DotFS; add sample file browser and move fs browse template to its own dir
- Add `failf` func; log initializer executions
- Adjust how terminal path patterns work
- Define app defaults in variables set via LDFLAGS; move docker build into ci.yaml with a test target
- Build and test caddy with the xtemplate plugin in CI

## [v0.6.5] - 2024-04-29

- Add JSON config option to set maxopenconns
- Add Config.Options() and Server.Stop()

## [v0.6.4] - 2024-04-08

- Add NATS request-reply functionality
- Restore go.work

## [v0.6.3] - 2024-04-07

- CI: cd into test to run tests

## [v0.6.2] - 2024-04-07

Consolidate modules and split cmd from app.

- Remove separate modules to consolidate lazy module loading
- Split cmd from app
- Only consider the root module tag as binary version; cmd uses local replacements
- Fix dockerfile

## [v0.6.1] - 2024-04-05

- Fix go.sum versions

## [v0.6.0] - 2024-04-05

Add the NATS provider and harden request/error handling.

- Add NATS and NATS KV providers (Publish, Subscribe, Get/Put/Delete/Purge/Watch)
- Add SendSSE method to DotFlush; update nats example
- Add basic multiuser chat demo
- Add a test command that runs and waits for SIGINT
- Use a private context key and add request id if missing; allow a special error to set the response status
- Split provider and values into separate files; simplify DotConfig marshalling
- Rename ConfigOverride to Option

## [v0.5.2] - 2024-03-31

- Fix git version identification

## [v0.5.1] - 2024-03-31

Expose the Go entrypoint and let .Resp serve files.

- Expose app.Main(), add version to binaries, fix docker build
- Refactor fs/resp so resp handles file serving
- Compress released zip file
- Isolate JSON coding newtypes

## [v0.5.0] - 2024-03-28

Introduce the dot provider system, JSON configuration, and the public Go API/docs.

- Create the dot provider system for customizing the template dot value; convert existing modules
- Accept JSON configuration (`--config` raw / `--config-file`), with round-trip config testing
- Add public Go API docs; expose all funcmap funcs so godoc is the primary documentation
- Factor out a builder to simplify Instance; remove Server/Instance interfaces and HandlerError
- Reorganize tests so hurl files correspond with template directories; add KV test
- Streamline README

## [v0.4.0] - 2024-03-11

Expose cleaner Instance and Server layers.

- Reorganize xtemplate to expose cleaner Config, Instance, and Server features (#3)
- Return markdown as HTML so it doesn't need the explicit `trustHtml` func
- Add library documentation; fix dockerfile

## [v0.3.3] - 2024-03-06

- Support minifying HTML templates at load time
- Set config defaults via a method; improve help output

## [v0.3.2] - 2024-03-05

- Simplify handlers; use LogAttrs; capture response metrics with httpsnoop
- Ping the db after opening it
- Point caddy at the new xtemplate-caddy module

## [v0.3.1] - 2024-02-22

- Upgrade deps; adjust default handling

## [v0.3.0] - 2024-02-22

Rewrite configuration, split out the caddy module, and add release automation.

- Rewrite config to use a simple struct; add Why and How-it-works sections to the README
- Remove the caddy module, to be republished as github.com/infogulch/xtemplate-caddy
- CI: cross-compile xtemplate and release on tag creation
- Ignore files hidden with '.' instead of '_'; allow a custom extension for template files
- Move tests to ./test; clean up CLI flags

## [v0.2.0] - 2024-02-10

Adopt Go 1.22's servemux, add SSE, and rewrite the README.

- Switch to Go 1.22, dropping the path matcher for the stdlib servemux
- Implement SSE with a client-side hot reload example
- Add instance cancellation; rename .SRI to .StaticFileHash and test static file serving
- Drop register packages; override configs on Main()
- Rewrite the README; add Dockerfile

## [v0.1.4] - 2023-10-19

Switch to functional options config and add encoding-negotiated static serving.

- Switch to functional options configuration style
- Refactor the static file handler; do encoding negotiation
- Add hurl integration tests
- Rename the bin package to cmd

## [v0.1.3] - 2023-10-13

- First pass at the integrated file server with cached SRI
- Use slog correctly; add request id; adjust logging
- Add a GitHub Actions CI job
- Fix route path mapping and a nil tx bug

## [v0.1.2] - 2023-10-12

- Wrap xtemplate into a simple CLI http server
- Clean up the caddy module and move caddy-specific docs
- Fix references to the previous repo name

## [v0.1.1] - 2023-10-09

- Fix return func

## [v0.1.0] - 2023-10-09

Initial standalone release, split out from caddy.

- Split xtemplate from caddy into standalone `xtemplate` / `xtemplate/caddy` packages
- Write a basic net/http server with a standalone demo
- Integrate a static file server with accept-encoding negotiation
- Split the watcher into a separate component; isolate caddy integration
- Refactor the router to return `http.Handler`; allow .ServeFile to serve from contextfs
- Make extrafuncs an array; switch to the functional options pattern
- Support SSE with a client-side hot reload demo
- Set up automation: build/upload binaries and hurl tests
