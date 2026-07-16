# Chat page renders
GET http://localhost:8080/

HTTP 200
[Asserts]
body contains "Chat"
body contains "EventSource"
body contains "bus"

# Publishing a message succeeds (browser EventSource round-trip is covered by
# lightpanda in test-example-sse-chat; multi-sub bus fan-out by providers/dotbus)
POST http://localhost:8080/messages
[FormParams]
msg: hello from hurl

HTTP 200
[Asserts]
body == "ok"

# The SSE endpoint requires the text/event-stream Accept header; without it the
# flushing handler refuses with 406.
GET http://localhost:8080/events

HTTP 406
