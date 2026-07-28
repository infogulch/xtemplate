GET http://localhost:8080/sse/test

HTTP 200
[Asserts]
body contains "sse-connect"

# Without an explicit text/event-stream Accept entry the flushing handler
# refuses with 406 (missing Accept / */* alone do not count).
GET http://localhost:8080/sse/events?count=1&delay=10

HTTP 406

# Exact EventSource-style Accept streams as text/event-stream.
GET http://localhost:8080/sse/events?count=11&delay=10
Accept: text/event-stream

HTTP 200
Content-Type: text/event-stream
Cache-Control: no-cache
[Asserts]
body contains "data: 10"

# Multi-type Accept lists (e.g. Datastar) also negotiate successfully.
GET http://localhost:8080/sse/events?count=3&delay=10
Accept: text/event-stream, text/html, application/json

HTTP 200
Content-Type: text/event-stream
[Asserts]
body contains "data: 2"
