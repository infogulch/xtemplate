
# TODO

## Features

- [ ] Split xtemplate from caddy so it can be used standalone
    - [ ] Split into separate packages `xtemplate` and `xtemplate/caddy`, rename repo to `xtemplate` (?)
    - [ ] Integrate a static file server based on `caddy.caddyhttp.file_server`

## Demos

- [ ] Demonstrate how to do auth with xtemplate
    - [ ] [forward_auth](https://caddyserver.com/docs/caddyfile/directives/forward_auth#forward-auth) / [Trusted Header SSO](https://www.authelia.com/integration/trusted-header-sso/introduction/)
- [ ] Demonstrate how to integrate with [caddy-git](https://github.com/greenpau/caddy-git) for zero-CI app deployments

# BACKLOG

- [ ] Investigate integrating into another web framework (gox/gin etc)

# DONE

- [x] Make extrafuncs an array
- Split xtemplate from caddy so it can be used standalone
    - [x] Isolate caddy integration into one file
