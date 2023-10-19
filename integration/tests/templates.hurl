# index
GET http://localhost:8080/visible

HTTP 200
[Asserts]
body contains "<p>You can't see me</p>"
