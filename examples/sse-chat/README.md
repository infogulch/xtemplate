# sse-chat

A live feed served over Server-Sent Events. The page opens an `EventSource`
to `/events`; the `{{define "SSE /events"}}` template pushes a bounded stream
of `data:` messages using `.Flush`, one every `delay` ms for `count`
iterations (query params, defaults 20 / 1000ms).

```sh
mise run example-sse-chat
```

Then open http://localhost:9002/

Extension idea: swap the server-generated counter for messages published to a
NATS subject for real multi-user chat (needs extra `nats` config; not done here).
