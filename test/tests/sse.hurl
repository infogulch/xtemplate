GET http://localhost:8080/sse/test

HTTP 200
[Asserts]
body contains "sse-connect"

# The SSE endpoint requires the text/event-stream Accept header; without it the
# flushing handler refuses with 406 (see flushingTemplateHandler).
GET http://localhost:8080/sse/events?count=1&delay=10

HTTP 406

# With the right Accept header it streams events as text/event-stream.
GET http://localhost:8080/sse/events?count=11&delay=10
Accept: text/event-stream

HTTP 200
Content-Type: text/event-stream
Cache-Control: no-cache
[Asserts]
body contains "data: 10"
