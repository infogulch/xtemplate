# dotprovider example

A custom dot provider (the repository pattern): a small Go type exposes
hardcoded in-memory data to templates as `{{.Shop}}`. `templates/index.html`
ranges over `{{.Shop.Products}}` and looks one up with `{{.Shop.Product 2}}`.

Run it:

```
mise run example-dotprovider
```

Then open http://localhost:9006/.

## Writing a dot provider

Implement `xtemplate.DotConfig` (`FieldName() string`, `Init(context.Context) error`,
`Value(xtemplate.Request) (any, error)`), then register the instance via
`xtemplate.WithProvider(p)` passed to `app.Main`. `FieldName` is the dot field
name (`Shop` here); `Value` returns the value assigned to `{{.Shop}}` for each
request. See `main.go`.
