# How to create a custom dot provider

If templates need to access something not covered by the [core providers](../reference/glossary.md#providers), implement a dot provider: a component that contributes one dot field to the per-request [dot context](../reference/dot-context.md) (`{{.Field}}`).

A complete runnable example: [`examples/dotprovider`](../../examples/dotprovider/).

## Ways to attach a provider

Every path implements the same `Provider` interface. Pick by how the provider is configured:

- **Custom provider** - construct it in Go and pass `WithProvider` (or put it on `Config.Providers`). Best for app-specific code next to `main`. Not selectable from JSON or Caddyfile.
- **Provider package** - self-register a type from `init()` so JSON can decode `"type"`. Required when the binary must resolve providers from config (CLI or Caddy JSON).
- **Caddyfile provider** - a Caddy module under `xtemplate.providers.*` that parses `provider <type> <field> { }` into JSON.

## 1. Implement Provider

Every dot provider has a field name, a prototype for type inference, and a per-request value step:

```go
type Provider interface {
	FieldName() string                    // name of the dot field, e.g. "Shop" → {{.Shop}}
	Prototype() any                       // non-nil typed zero; used only for reflection at instance build
	Value(http.ResponseWriter, *http.Request) (any, error) // once per request; assigned to the dot field
}
```

```go
// shopProvider is the provider config (here: no extra settings).
type shopProvider struct{}

func (shopProvider) FieldName() string                         { return "Shop" }
func (shopProvider) Prototype() any                            { return Shop{} }
func (shopProvider) Value(http.ResponseWriter, *http.Request) (any, error) { return Shop{}, nil }
```

- `FieldName` chooses the dot field on the dot context. Two providers on one instance must not share a field name.
- `Prototype` must return a non-nil value of the same concrete type that `Value` returns. It is discarded after type inference.
- Methods on the value returned by `Value` are callable from templates (`{{.Shop.Product 1}}`).

Optional: implement `Initializer` for instance-scoped setup (open connections, validate config). The instance context is cancelled on reload/stop; retain it on the provider if request-time code must observe reload/stop (do not expect it on `Value`).

```go
type Initializer interface {
	Init(context.Context) error // save ctx on the provider if needed later
}
```

Optional: implement `Finalizer` for per-request teardown after template execution (commit / rollback, close handles, write buffered status). The core SQL provider uses this.

```go
type Finalizer interface {
	Finalize(v any, err error) error // v is the value Value returned for the request
}
```

Optional: implement `Closer` to release instance-scoped resources when the instance is retired (reload or stop). Prefer this when the provider owns connections; context cancellation alone is easy to forget.

```go
type Closer interface {
	Close() error
}
```

## 2a. Custom provider: `WithProvider`

A custom provider is user Go code attached to the config without appearing in the type registry:

```go
app.Main(xtemplate.WithProvider(shopProvider{}))
// also: watchfs.Main(...), git.Main(...), or
// cfg.Server(xtemplate.WithProvider(...)) / cfg.Instance(...)
// or append to Config.Providers
```

On instance load, xtemplate builds the dot context struct to include your field. On every request, `Value` runs before the template executes.

No JSON `"type"` is involved: the provider is already a constructed `Provider` value.

## 2b. Provider package: xtemplate registry (`providers.go`)

To configure the provider from JSON (`"providers": [{ "type": "…" }]`) which supports both xtemplate CLI and Caddy JSON config, in a provider package, call `xtemplate.Register` with a value that implements `xtemplate.Provider` and supports go JSON [un]marshalling:

```go
package shop

import (
	"net/http"

	"github.com/infogulch/xtemplate"
)

func init() {
	// "shop" is the provider type - JSON discriminator and Caddyfile token.
	xtemplate.RegisterProvider("shop", func() xtemplate.Provider {
		return &ShopConfig{}
	})
}

// ShopConfig is the provider config: type-specific settings + field name.
type ShopConfig struct {
	Name string `json:"name"` // dot field name, e.g. "Shop"
	// … other settings …
}

func (c *ShopConfig) FieldName() string { return c.Name }
func (c *ShopConfig) Prototype() any    { return Shop{} }
func (c *ShopConfig) Value(http.ResponseWriter, *http.Request) (any, error) {
	return Shop{}, nil
}
```

Here's how the registry works:

1. `RegisterProvider(name, ctor)`: maps a provider type string to a constructor. Call only from `init()`; duplicate names panic.
2. At instance build, `resolveProviders` peeks each entry’s `"type"`, looks up the constructor, re-decodes into the concrete type, and returns `[]Provider` for the instance. See [`providers.go`](../../providers.go).

Opt the type into a binary by blank-importing the package (same pattern as the standard provider set under `cmd`):

```go
import _ "example.com/myapp/providers/shop"
```

Once registered and imported:

```json
{
  "providers": [
    { "type": "shop", "name": "Shop" }
  ]
}
```

Unknown `"type"` fails at instance build with a hint to import the package that registers it if it matches the name of a core provider.

## 2c. Caddyfile provider: `provider <type> <field> { }`

To configure a provider from a Caddyfile `provider <type> <field> { }` block, register a Caddy module under `xtemplate.providers.*` that parses the block into a `json.RawMessage`. Dispatch injects `"type"` and `"name"`; instance load still decodes that JSON through the xtemplate type registry. The two registries are independent and only share the JSON intermediate form.

Mirror the layout under `providers/dotsql/caddyfile/` and blank-import it in your Caddy build. See [`caddy/README.md`](../../caddy/README.md) and [ADR 0003](../adr/0003-caddyfile-provider-dispatch-via-caddy-module-registry.md).

## 3. Complete custom-provider example

```go
package main

import (
	"net/http"

	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/app"
)

type Product struct {
	ID    int
	Name  string
	Price string
}

// Shop is the value assigned to the dot field (e.g. {{.Shop}}).
type Shop struct{}

func (Shop) Products() []Product {
	return []Product{{1, "Widget", "$9.99"}}
}

type shopProvider struct{}

func (shopProvider) FieldName() string { return "Shop" }
func (shopProvider) Prototype() any    { return Shop{} }
func (shopProvider) Value(http.ResponseWriter, *http.Request) (any, error) {
	return Shop{}, nil
}

func main() {
	app.Main(xtemplate.WithProvider(shopProvider{}))
}
```

```html
{{range .Shop.Products}}
  <li>{{.Name}}: <em>{{.Price}}</em></li>
{{end}}
```

See [`examples/dotprovider`](../../examples/dotprovider/) for the full program and tests. Core provider packages under [`providers/`](../../providers/) show registry + JSON field tags in production form.

## Related

- [Glossary - Providers](../reference/glossary.md#providers)
- [Dot context](../reference/dot-context.md)
- [Configuration](../reference/configuration.md)
- [Custom build](custom-build.md) - blank-import provider packages into a binary
- [ADR 0001 - Global init registry](../adr/0001-global-init-registry-for-dot-providers.md)
- [ADR 0003 - Caddyfile provider dispatch](../adr/0003-caddyfile-provider-dispatch-via-caddy-module-registry.md)
- [ADR 0006 - Reflection-assembled dot context](../adr/0006-reflection-assembled-dot-context.md)
