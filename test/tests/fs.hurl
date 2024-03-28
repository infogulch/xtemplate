# reading files from fs
GET http://localhost:8080/fs

HTTP 200
[Asserts]
body contains "<!doctype html>"
body contains "bar"
