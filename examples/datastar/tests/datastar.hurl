# Landing page serves Datastar from static assets and opens a live stream.
# (Minify may drop quotes around simple attribute values: id=counter.)
GET http://localhost:8080/

HTTP 200
[Asserts]
body contains "Datastar + xtemplate"
body contains "datastar.js"
body contains "id=counter"
body contains "id=count"
body contains "@post('/inc')"
body contains "@post('/dec')"
body contains "@get('/stream'"
body contains "data-init="
body contains "openWhenHidden"

# Datastar library is hosted as a static file (not a CDN).
GET http://localhost:8080/assets/datastar.js

HTTP 200
[Asserts]
header "Content-Type" contains "javascript"
body contains "Datastar"

# /stream is SSE-only: missing Accept / */* alone → 406. (Multi-type Accept
# negotiation is covered by unit tests and test/tests/sse.hurl; do not GET the
# live bus stream here — it never ends.)
GET http://localhost:8080/stream

HTTP 406

# Increment publishes on the bus and returns ok (does not paint via SSE).
POST http://localhost:8080/inc

HTTP 200
[Asserts]
body == "ok"

# Shared value is visible on a full page load after the mutation.
GET http://localhost:8080/

HTTP 200
[Asserts]
body contains ">1<"

# Decrement publishes and returns ok.
POST http://localhost:8080/dec

HTTP 200
[Asserts]
body == "ok"

GET http://localhost:8080/

HTTP 200
[Asserts]
body contains ">0<"
