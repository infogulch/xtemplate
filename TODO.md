# TODO

- [ ] Republish caddy module as github.com/infogulch/xtemplate-caddy

### Automation

- Add github workflows
    - [ ] Set up go tests
    - [ ] Publish binaries and release when a git tag is pushed

### Application

- [ ] Make and link to more example applications
- [ ] Demonstrate how to do auth with xtemplate
    - [ ] [forward_auth](https://caddyserver.com/docs/caddyfile/directives/forward_auth#forward-auth) / [Trusted Header SSO](https://www.authelia.com/integration/trusted-header-sso/introduction/)

# BACKLOG

- Support SSE
    - [ ] Integrate nats subscription
- [ ] Add a way to register additional routes dynamically during init
- [ ] Organize docs according to https://diataxis.fr/
- [ ] Demonstrate how to integrate with [caddy-git](https://github.com/greenpau/caddy-git) for zero-CI app deployments

# DONE

## v0.2 - Feb 2024

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
