# NATS endpoints that return a normal, terminating response. The SSE stream at
# GET /nats/messages is intentionally excluded — see nats.hurl.disabled.

# the chat page renders
GET http://localhost:8080/nats/
HTTP 200
[Asserts]
body contains "multi-user realtime chat"


# the bare path redirects to the trailing-slash canonical path
GET http://localhost:8080/nats
HTTP 301
[Asserts]
header "Location" == "/nats/"


# publishing a message returns a fresh message input fragment
POST http://localhost:8080/nats/messages
[FormParams]
msg: hello
HTTP 200
[Asserts]
body == "<input autofocus name=msg required>"
