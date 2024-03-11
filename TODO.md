# TODO

- [ ] Create system for optional modules. DB/FS/NATS. Inject?
- [ ] Integrate nats:
    - [ ] Subscribe to subject, loop on receive to send via open SSE connection
    - [ ] Publish message to subject
    - [ ] Request-Reply

### Automation

- [ ] Write Go api tests
- [ ] Write CLI tests

### Application

- [ ] Make and link to more example applications
    - [ ] Demo/test how to use sql
    - [ ] Demo/test reading and writing to the context fs
- [ ] Demonstrate how to do auth with xtemplate
    - [ ] [forward_auth](https://caddyserver.com/docs/caddyfile/directives/forward_auth#forward-auth) / [Trusted Header SSO](https://www.authelia.com/integration/trusted-header-sso/introduction/)
- [ ] Demo integration with [caddy-git](https://github.com/greenpau/caddy-git) for zero-CI app deployments

# BACKLOG

- [ ] Add a way to register additional routes dynamically during init
- [ ] Organize docs according to https://diataxis.fr/
- [ ] Fine tune timeouts? https://ieftimov.com/posts/make-resilient-golang-net-http-servers-using-timeouts-deadlines-context-cancellation/
- [ ] Idea: Add special FILE pseudo-func that is replaced with a string constant of the current filename.
    - Potentially useful for invoking a template file with a relative path. (Add DIR constant too?)
    - Parse().Tree.Root.(*ListNode).[].(recurse) where NodeType()==NodeIdentifier replace with StringNode
    - Should be fine?
- [ ] Add command that pre-compresses static files
- [ ] Pass Config.Ctx down to http.Server/net.Listener to allow caller to cancel
  .Serve() and all associated instances.

# DONE

## v0.4 - Mar 2024

- [x] Reorganize to expose different layers:
    - Config: Configure xtemplate
    - Instance: Serves as the local context and http.Handler
    - Server: Manages instances and lets you live reload live.

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
