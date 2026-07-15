# xtemplate examples

Small, self-contained apps that each demonstrate one xtemplate feature.

The quickest way to explore them is to start everything at once:

```sh
mise run examples
```

This launches all six examples plus a landing page at
<http://localhost:9000> with links to each one. Press Ctrl+C to stop them all.

If one of those ports is already taken, pass a different base port; everything
shifts up from there (the hub takes the base, each example the base + its
offset):

```sh
mise run examples 9100
```

To run just one as a dev server instead, use `mise run example-<name>` and open
the listed URL.

| Example | Feature | Run task | Port | URL |
|---|---|---|---|---|
| [`contacts`](./contacts/) | File-based routing + custom method routes + `.DB` (sqlite CRUD) | `mise run example-contacts` | 9001 | <http://localhost:9001/> |
| [`sse-chat`](./sse-chat/) | Server-Sent Events with `.Flush` (live updates) | `mise run example-sse-chat` | 9002 | <http://localhost:9002/> |
| [`blog`](./blog/) | Optimal asset serving: content-hash cache-busting + Subresource Integrity (`.X.StaticFileHash`) | `mise run example-blog` | 9003 | <http://localhost:9003/> |
| [`filebrowser`](./filebrowser/) | Filesystem dot provider (`.FS` list/read + `ReceiveFiles` upload) | `mise run example-filebrowser` | 9004 | <http://localhost:9004/> |
| [`embedded`](./embedded/) | Single-binary deployment with `//go:embed` (custom build) | `mise run example-embedded` | 9005 | <http://localhost:9005/> |
| [`dotprovider`](./dotprovider/) | Custom dot provider exposing Go code to templates (custom build) | `mise run example-dotprovider` | 9006 | <http://localhost:9006/> |

## How these are wired

- **Examples 1-4** (`contacts`, `sse-chat`, `blog`, `filebrowser`) run the
  default CLI binary built by `mise run build-cli` (`dist/xtemplate`) against a
  `config.json` in the example directory.
- **Examples 5-6** (`embedded`, `dotprovider`) are custom Go programs (their own
  `main.go` calling `app.Main(...)`) and have their own `build-example-<name>`
  task.

Each example has:

- A `templates/` directory (the web root). 
- A `tests/*.hurl` smoke test. Following the convention of the main suite, the
  `.hurl` files hardcode `localhost:8080`; the harness remaps that to the
  example's real port with `curl --connect-to`, so the same files work no matter
  which port the example listens on.

## Tasks

- `mise run example-<name>` - run the example as a foreground dev server.
- `mise run test-example-<name>` - build, serve on its port, run its hurl suite, tear down.
- `mise run test-examples` - run every example's integration test (included in `mise run test` / `mise run ci`).
