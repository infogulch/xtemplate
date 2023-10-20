GET http://localhost:8080/sse

HTTP 200
[Asserts]
body contains "sse-connect"

GET http://localhost:8080/events/2

HTTP 200
[Asserts]
body contains "data: 1"
