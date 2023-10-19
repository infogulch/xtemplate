# TODO

### Features

- [ ] Refactor router to return `http.Handler`, use custom handler for static files

### Documentation

- [ ] Highlight file server feature
- [ ] Add "Why?" section to readme.
- [ ] Organize docs according to https://diataxis.fr/

### Automation

- Add github workflows
    - [ ] Set up go tests
    - [ ] Publish binaries and release when a git tag is pushed

### Demos

- [ ] Demonstrate how to do auth with xtemplate
    - [ ] [forward_auth](https://caddyserver.com/docs/caddyfile/directives/forward_auth#forward-auth) / [Trusted Header SSO](https://www.authelia.com/integration/trusted-header-sso/introduction/)
- [ ] Demonstrate how to integrate with [caddy-git](https://github.com/greenpau/caddy-git) for zero-CI app deployments


# BACKLOG

- [ ] Client side auto reload
- [ ] Investigate integrating into another web framework (gox/gin etc)
- [ ] Document how to use standalone
- [ ] Demo how to use standalone
- [ ] Build a way to send live updates to a page by rendering a template to an SSE stream. Maybe backed by NATS.io?
- [ ] Consider using the functional options pattern for configuring XTemplate
- [ ] Convert *runtime to an `atomic.Pointer[T]`
- [ ] Allow .ServeFile to serve files from contextfs
- [ ] Switch to using 1.22's new servemux


# DONE

## v0.1

- [x] Make extrafuncs an array
- [x] Split xtemplate from caddy so it can be used standalone
    - [x] Split watcher into a separate component
    - [x] Isolate caddy integration into one file
    - [x] Split into separate packages `xtemplate` and `xtemplate/caddy`, rename repo to `xtemplate`
    - [x] Write basic server based on net/http
    - [x] Update docs describe the separate packages
    - [x] Integrate a static file server
        - [x] Change file server to actually negotiate accept-encoding
- [x] Set up automation
    - [x] Build and upload binaries
    - [x] Set up hurl tests
