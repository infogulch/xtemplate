# sse-chat

Multi-user realtime chat over Server-Sent Events and the in-process
[`bus`](../../docs/reference/dot-context.md#in-process-broadcast-with-bus)
provider.

- The page opens an `EventSource` to `/events`, which ranges
  `{{.Bus.Subscribe "messages"}}` and streams each message with `.Flush.SendSSE`.
- The form `POST`s to `/messages`, which calls
  `{{.Bus.Publish "messages" …}}`.
- Open two browser tabs to see messages fan out to every connected client.

```sh
mise run example-sse-chat
```

Then open http://localhost:9002/

For multi-process messaging (or request/reply, durable streams), swap `bus`
for the [`nats`](../../docs/reference/dot-context.md#messaging-with-nats)
provider instead.
