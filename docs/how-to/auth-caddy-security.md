# Add authentication with caddy-security

xtemplate does not implement login itself. Run it as a Caddy handler and put an auth module in front of the routes that must be protected. A common choice is [AuthCrunch caddy-security](https://authcrunch.com/) (`github.com/greenpau/caddy-security`).

## Pattern

1. Build Caddy with both `xtemplate` (standard or lean) and `caddy-security`.
2. Configure an authentication portal / authorizer in the Caddyfile.
3. Wrap only the routes that need auth; leave public static files and login endpoints outside the authorizer.

Conceptual Caddyfile shape (consult caddy-security docs for exact directives relevant to your version):

```Caddyfile
{
	order authenticate before respond
	order authorize before basicauth

	security {
		# portal / oauth / credentials; see AuthCrunch docs
	}
}

:443 {
	route /login* {
		authenticate with <portal_name>
	}

	route {
		authorize with <policy_name>
		xtemplate {
			templates_dir templates
		}
	}
}
```

Build sketch:

```shell
xcaddy build \
  --with github.com/infogulch/xtemplate/caddy/standard \
  --with github.com/greenpau/caddy-security
```

## What templates see

After a successful authorize step, identity often appears in headers or Caddy placeholders. Forward what you need into templates via:

- request headers (read with `.Req.Header.Get`), or
- a small [custom provider](create-a-provider.md) that parses the auth context

Keep secrets and token validation in Caddy (or your IdP); templates should only consume already-verified identity claims.

## Related

- [Deployment modes → Caddy](../reference/deployment-modes.md#caddy-module)
- [`caddy/README.md`](../../caddy/README.md)
- [AuthCrunch documentation](https://docs.authcrunch.com/)
