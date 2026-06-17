GET http://localhost:8080/

HTTP 200
[Asserts]
body contains "Live feed"
body contains "EventSource"

# The SSE endpoint requires the text/event-stream Accept header; without it the
# flushing handler refuses with 406.
GET http://localhost:8080/events?count=1&delay=10

HTTP 406

# With the right Accept header it streams events as text/event-stream.
GET http://localhost:8080/events?count=3&delay=10
Accept: text/event-stream

HTTP 200
Content-Type: text/event-stream
[Asserts]
body contains "data: message 0"
