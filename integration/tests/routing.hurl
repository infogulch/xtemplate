# index
GET http://localhost:8080/

HTTP 200
[Asserts]
body contains "<p>Hello world!</p>"

# subdir
GET http://localhost:8080/subdir

HTTP 200
[Asserts]
body contains "subdir"

GET http://localhost:8080/subdir/file

HTTP 200
[Asserts]
body contains "hello!"
