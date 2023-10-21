# TODO

### Features


### Documentation

- [ ] Memes
- [ ] Highlight file server feature
- [ ] Highlight sse feature
- [ ] Organize docs according to https://diataxis.fr/
    - [ ] Add explanation
- [ ] Document configuration

### Automation

- Add github workflows
    - [ ] Set up go tests
    - [ ] Publish binaries and release when a git tag is pushed

### Demos

- [ ] Demonstrate how to do auth with xtemplate
    - [ ] [forward_auth](https://caddyserver.com/docs/caddyfile/directives/forward_auth#forward-auth) / [Trusted Header SSO](https://www.authelia.com/integration/trusted-header-sso/introduction/)
- [ ] Demonstrate how to integrate with [caddy-git](https://github.com/greenpau/caddy-git) for zero-CI app deployments


# BACKLOG

- [ ] Switch to using Go 1.22's new servemux
    - [ ] Add PathValue method to .Req (future proofing)
- Support SSE
    - [ ] Integrate nats subscription
- [ ] Split caddy integration into a separate repo. Trying to shoehorn two modules into one repo just isn't working.
- [ ] Add a way to register additional routes dynamically during init


# DONE

## v0.1

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
