# TODO

- [ ] Accept JSON
    - [x] Add config flag to load config from JSON file
    - [x] Allow raw config with --config and config file with --config-file
    - [x] Parse args -> decode config files in args to args -> decode config
      values in args to args -> parse args again
    - [ ] Test that everything can be configured, load config -> dump back
    - [ ] Validate that your type is correct on call to Value
- [ ] Dot Provider system
    - [ ] Accept configuration from JSON
    - [ ] Update `xtemplate-caddy`. Note only caddy 2.8.0 uses Go 1.22
        - [ ] Must test on caddy head?
        - [ ] Accept dot provider configuration from Caddyfile
    - [ ]
- [ ] Add/update documentation
    - [ ] Creating a provider
    - [ ] Using the new go-arg cli flags
- [ ] Expose all funcmap funcs so godoc can be the primary documentation.
- [ ] Add ServeTemplate that delays template rendering until requested by
  http.ServeContent to optimize cache behavior. Something like
  https://github.com/spatialcurrent/go-lazy ?
- [ ] Add NATS module:
    - [ ] Subscribe to subject, loop on receive to send via open SSE connection
    - [ ] Publish message to subject
    - [ ] Request-Reply
- [ ] Add mail module:
    - [ ] Send mail, send mail by rendering template
- [ ] Use https://github.com/abhinav/goldmark-frontmatter
- [ ] Publish docker image, document docker usage
- [ ] Pass Config.Ctx down to http.Server/net.Listener to allow caller to cancel
  .Serve() and associated instances.

### Testing

- [ ] Test configuration methods
    - [ ] JSON
    - [ ] CLI
    - [ ] Go API
    - [ ] Test provider configuration
- [ ] Test all built-in and optional dot providers

### Application

- [ ] Measure performance
- [ ] Make and link to more example applications
    - [ ] Demo/test how to use sql
    - [ ] Demo/test reading and writing to the context fs
- [ ] Demonstrate how to do auth with xtemplate
    - [ ] [forward_auth](https://caddyserver.com/docs/caddyfile/directives/forward_auth#forward-auth) / [Trusted Header SSO](https://www.authelia.com/integration/trusted-header-sso/introduction/)
- [ ] Demo integration with [caddy-git](https://github.com/greenpau/caddy-git) for zero-CI app deployments

# BACKLOG

- [ ] Accept Env configuration
- [ ] Built-in CSRF handling?
- [ ] Fine tune timeouts? https://ieftimov.com/posts/make-resilient-golang-net-http-servers-using-timeouts-deadlines-context-cancellation/
- [ ] Idea: Add special FILE pseudo-func that is replaced with a string constant of the current filename.
    - Potentially useful for invoking a template file with a relative path. (Add
      DIR constant too?)
    - Parse().Tree.Root.(*ListNode).[].(recurse) where NodeType()==NodeIdentifier replace with StringNode
- [ ] Modify relative path invocations to point to the local path. https://pkg.go.dev/text/template/parse@go1.22.1#TemplateNode
    - Should be fine?
- [ ] See if its possible to implement sql queryrows with https://go.dev/wiki/RangefuncExperiment
    - Not until caddy releases 2.8.0 and upgrades to 1.22.
- [ ] Add command that pre-compresses static files
- [ ] Schema migration? https://david.rothlis.net/declarative-schema-migration-for-sqlite/
- [ ] Schema generator: https://gitlab.com/Screwtapello/sqlite-schema-diagram/-/blob/main/sqlite-schema-diagram.sql?ref_type=heads
- [ ] Add a way to register additional routes dynamically during init
- [ ] Organize docs according to https://diataxis.fr/

# DONE

## Next

- Accept JSON configuration
    - [x] Implement Json Unmarshaller https://pkg.go.dev/encoding/json
- [-] Downgrade to go 1.21 - Cannot due to using 1.22 ServeMux
- Add/update documentation
    - [x] Readme
    - [x] Provider Go API docs
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
