# Test CORS protection with untrusted origin
OPTIONS http://localhost:8080/
Origin: http://evil.com
Access-Control-Request-Method: GET

HTTP 405

# Test CORS protection with simple cross-origin request
GET http://localhost:8080/
Origin: http://evil.com

HTTP 200
[Asserts]
header "Access-Control-Allow-Origin" not exists

# Test preflight with trusted origin (but none are trusted, so 405)
# Since no trusted origins, even "null" or same should be protected if specified.
# But for localhost:8080, if hurl is localhost:8080 to server localhost:8080, same origin, no protection needed.
# But to test, using cross-origin.

# Test OPTIONS preflight with Access-Control-Request-Method
OPTIONS http://localhost:8080/assets/file.txt
Origin: http://example.com
Access-Control-Request-Method: GET

HTTP 405

# Test GET with cross-origin
GET http://localhost:8080/assets/file.txt
Origin: http://example.com

HTTP 200
Content-Type: text/plain; charset=utf-8
[Asserts]
header "Access-Control-Allow-Origin" not exists
body == "testing"
