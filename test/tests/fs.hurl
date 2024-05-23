# reading files from fs
GET http://localhost:8080/fs/

HTTP 200
[Asserts]
body contains "<!doctype html>"
body contains "listing"

# serve content
GET http://localhost:8080/fs/serve

HTTP 200
Content-Type: text/plain; charset=utf-8
[Asserts]
body not contains "doctype"
body contains "bar"

# openclose
GET http://localhost:8080/fs/openclose

HTTP 200
