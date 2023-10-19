# calling another template
GET http://localhost:8080/visible

HTTP 200
[Asserts]
body contains "<p>You can't see me</p>"


# reading files from context
GET http://localhost:8080/context

HTTP 200
[Asserts]
body contains "<!DOCTYPE html>"
body contains "bar"


# reading files from context, define a handler inline
GET http://localhost:8080/hello

HTTP 200
[Asserts]
body contains "world"
