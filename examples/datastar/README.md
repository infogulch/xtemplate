# datastar

Reactive UI with [Datastar](https://data-star.dev/) over xtemplate’s
[`SSE`](../../docs/reference/instance-loading.md) routes,
[`.Flush.SendSSE`](../../docs/reference/dot-context.md#streaming-control-in-flush),
and the in-process [`bus`](../../docs/reference/dot-context.md#in-process-broadcast-with-bus)
provider — same split as [`sse-chat`](../sse-chat/): mutations publish, the
stream paints.

- The page loads [`datastar.js`](./templates/assets/datastar.js) as a static
  file from the templates directory (self-hosted, not a CDN).
- `data-init` opens a long-lived `@get('/stream')` SSE connection
  (`openWhenHidden: true`) that ranges `{{.Bus.Subscribe "counter"}}` and
  morphs `#counter` on each message. Use `data-init`, not `data-on:load`.
- `+1` / `−1` call `@post('/inc')` / `@post('/dec')`: update SQLite, publish
  the rendered patch on the bus, return `ok`. They do **not** send SSE.
- Every open tab (including the clicker) updates only via `/stream`.
- No hand-written `EventSource` JavaScript.

```sh
mise run example-datastar
```

Then open http://localhost:9007/ in two tabs and click in either.
