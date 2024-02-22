GET http://localhost:8080/sse

HTTP 200
[Asserts]
body contains "sse-connect"

GET http://localhost:8080/events?count=11&delay=10
Accept: text/event-stream

HTTP 200
[Asserts]
body contains "data: 10"
