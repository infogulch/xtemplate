# TODO

- [ ] Accept dot provider configuration from Caddyfile
- [ ] Add .TemplateLazy that renders a template to a io.ReadSeeker after the
  first call to a method. Can be used for mail, servecontent, etc
  - https://github.com/spatialcurrent/go-lazy ?
- [ ] Add mail module:
  - [ ] Send mail, send mail by rendering template
  - https://github.com/Shopify/gomail
- [ ] Use https://github.com/abhinav/goldmark-frontmatter
- [ ] Pass Config.Ctx down to http.Server/net.Listener to allow caller to cancel
  .Serve() and associated instances.

### Testing

- [ ] Test configuration methods
  - [ ] CLI
  - [ ] Go API
- [ ] Test dot providers
  - [ ] FS
  - [ ] DB

### Documentation

- [ ] Using different databases
  - Should be documented with DotDBProvider go docs (?)
- [ ] Using the new go-arg cli flags
- [ ] Using json config
- [ ] Creating a provider
- [ ] Document docker usage

### Application

- [ ] Measure performance
- [ ] Make and link to more example applications
  - [ ] Demo/test how to use sql
  - [ ] Demo/test reading and writing to the fs dot provider
- [ ] Demonstrate authentication with xtemplate
  - [ ] [forward_auth](https://caddyserver.com/docs/caddyfile/directives/forward_auth#forward-auth) / [Trusted Header SSO](https://www.authelia.com/integration/trusted-header-sso/introduction/)

# BACKLOG

- [ ] Look into https://github.com/42atomys/sprout
- [ ] Review https://github.com/hairyhenderson/gomplate for data source ideas
- [ ] Fix `superfluous response.WriteHeader call from github.com/felixge/httpsnoop.(*Metrics).CaptureMetrics` https://go.dev/play/p/spBB4w7nBCZ
- [ ] Fine tune timeouts? https://ieftimov.com/posts/make-resilient-golang-net-http-servers-using-timeouts-deadlines-context-cancellation/
- [ ] Idea: Add special FILE pseudo-func that is replaced with a string constant of the current filename.
  - Potentially useful for invoking a template file with a relative path. (Add
    DIR constant too?)
  - Parse().Tree.Root.(*ListNode).[].(recurse) where NodeType()==NodeIdentifier replace with StringNode
- [ ] Modify relative path invocations to point to the local path. https://pkg.go.dev/text/template/parse@go1.22.1#TemplateNode
  - Should be fine?
- [ ] See if its possible to implement sql queryrows with https://go.dev/wiki/RangefuncExperiment
  - Not until caddy releases 2.8.0 and upgrades to 1.22.
- [ ] Add a way to register additional routes dynamically during init
- [ ] Organize docs according to https://diataxis.fr/
- [ ] Research alternative template loading strategies:
  - https://github.com/claceio/clace/blob/898932c1766d3e063c67caf1b9a744777fa437c3/internal/app/app.go#L261
  - https://github.com/unrolled/render

# DONE

## next

- [ ] Enable writable file system for template fs and fs dot provider; fixes #36
- [ ] Add option to pre-compresses static files
- [ ] Built-in CSRF handling with nosurf; fixes #49
- [ ] Integrate datastar functions
- [ ] Load templates from a remote git repo, updating automatically when new commits are pushed; fixes #46

## v0.8.4

- [x] Use zed agent to summarize and generate release notes
- [x] Mention template embedding in features list, fixes #50
- [x] Update readme repository layout section for caddy module changes
- [x] Add go-arg help epilogue examples to match the readme
- [x] Add link tag template test
- [x] Relax match rule
- [x] Add hello world example
- [x] Remove migration scripts
- [x] VSCode: listen on 8080 in launch.json; use CGO; add "run hurl" task, fixes #44
- [x] Add debug launch profiles for xtemplate and caddy

## v0.8.1-v0.8.3

- [x] Update dependencies
- [x] Add logging to caddy tester; always upload logs in GH CI
- [x] Build before zipping, fixes #29; add flag to build caddy with debug symbols
- [x] Update README CLI flags listing; fixes #30
- [x] Fix static file hash example, fixes #31
- [x] CI improvements: fix version command, simplify testing, allow docker push to fail
- [x] VSCode setup to debug test folder

## v0.6.1-v0.8.0

- [x] Split cmd from app into separate modules for better organization
- [x] Add NATS request-reply functionality to NATS provider
- [x] Add Config.Options() and Server.Stop() methods for better API control
- [x] Add JSON config option to set maxopenconns for database connections
- [x] Enhanced 'try' function to be able to call methods
- [x] Fix Instance.ServeHTTP so PathValue is set on request context
- [x] Add Dir method to DotFS for directory operations
- [x] Add sample file browser template and move fs browse template to own directory
- [x] Fix invoking INIT templates with proper dot values
- [x] Major refactor to simplify dot provider configuration system
- [x] Remove separate modules to consolidate lazy module loading
- [x] Improve build system with LDFLAGS for app defaults and Docker CI integration

## v0.6.0 - Apr 2024

- Rename ConfigOverride to Option
- Add NATS provider:
  - [x] Publish, Subscribe to subject, loop on receive to send via open SSE connection
  - [x] Add basic multiuser chat demo
  - [x] KV: Get/Put/Delete/Purge/Watch

## v0.5.2 - Mar 2024

- Documentation
  - [x] Fix readme to include links to the godoc page
  - [x] Remove markdown `code` sections from godoc comments since they are not supported
  - [x] Fix godoc formatting in general: https://pkg.go.dev/github.com/fluhus/godoc-tricks
- [x] Add version information to binaries
- [x] Fix docker build, add to ci
- [x] Expose Main() so Go lib users can use it, moving cmd to app/cmd
- Documentation
  - [x] Fix formatting; see https://pkg.go.dev/github.com/fluhus/godoc-tricks
- [x] Have .Resp handle response serving for fs files.

## v0.5 - Mar 2024

- Tag v0.5.0
- Tests
  - [x] Reorganize tests so hurl files correspond with directories in templates
  - [x] Test provider configuration
  - [x] JSON
  - Test dot providers
    - [x] KV
- [x] Accept JSON configuration
  - [x] Implement Json Unmarshaller https://pkg.go.dev/encoding/json
  - [x] Add config flag to load config from JSON file
  - [x] Allow raw config with --config and config file with --config-file
  - [x] Parse args -> decode config files in args to args -> decode config
    values in args to args -> parse args again
  - [x] Test that everything can be configured, load config -> dump back
- [-] Downgrade to go 1.21 - Cannot due to using 1.22 ServeMux
- Documentation
  - [x] Streamline readme
  - [x] Provider Go API docs
  - [x] Expose all funcmap funcs so godoc can be the primary documentation.
- Dot Provider system
  - [x] Create system for customizing template dot value
  - [x] Convert existing modules
  - [x] Accept configuration from cli
  - [x] Figure out how to document providers: Use go docs
- [x] Get rid of Server/Instance interfaces, expose structs directly
- [x] Catch servemux addhandler panics and return an error instead
- [x] Use go-arg library for arg parsing
    - [x] Fix go-arg embedded structs or don't use them https://github.com/alexflint/go-arg/issues/242

## v0.4 - Mar 2024

- [x] Add library documentation
- [x] Reorganize to expose different layers:
  - Config: Configure xtemplate
  - Instance: Serves as the local context and http.Handler
  - Server: Manages instances and lets you live reload live.
- [x] Fix dockerfile
  - https://blog.2read.net/posts/building-a-minimalist-docker-container-with-alpine-linux-and-golang/
  - https://chemidy.medium.com/create-the-smallest-and-secured-golang-docker-image-based-on-scratch-4752223b7324

## v0.3 - Mar 2024

- [x] Refactor watch to be easier to use from both Main() and xtemplate-caddy.
- [x] Use LogAttrs in hot paths
- [x] Simplify handlers
- [x] Use github.com/felixge/httpsnoop to capture response metrics
- [x] Support minifying templates as they're loaded. https://github.com/tdewolff/minify
- [x] Adjust the way config defaults are set
- [x] Improve cli help output

## v0.3 - Feb 2024

- [x] Republish caddy module as github.com/infogulch/xtemplate-caddy
- [x] Add github workflow to publish binaries and release when a git tag is pushed
- [x] Rewrite configuration to be normal
- [x] Add Why and How to use sections to README
- [x] Remove caddy module, to republish as github.com/infogulch/xtemplate-caddy

## v0.2 - Feb 2024

- [x] Highlight file server feature
- [x] Switch to using Go 1.22's new servemux
  - [x] Add PathValue method to .Req
- [x] Allow truncated hash to positively identify file; switch to url-encoded hash value
- [x] Allow searching for custom file extension to identify template files
- [x] Don't route hidden files that start with a '.', not a '_'. We don't need to reinvent hidden files.
- [x] Fix docs, tests after `_` -> `.` change.

## v0.1 - Oct 2023

- [x] Make extrafuncs an array
- [x] Split xtemplate from caddy so it can be used standalone
  - [x] Split watcher into a separate component
  - [x] Isolate caddy integration into one file
  - [x] Split into separate packages `xtemplate` and `xtemplate/caddy`, rename repo to `xtemplate`
  - [x] Write basic server based on net/http
    - [x] Demo how to use standalone
  - [x] Update docs describe the separate packages
  - [x] Integrate a static file server
    - [x] Negotiate accept-encoding
- [x] Set up automation
  - [x] Build and upload binaries
  - [x] Set up hurl tests
- [x] Refactor router to return `http.Handler`, use custom handler for static files
- [x] Allow .ServeFile to serve files from contextfs
- [x] Switch to functional options pattern for configuration
- [x] Support SSE
  - [x] Demo client side hot reload
