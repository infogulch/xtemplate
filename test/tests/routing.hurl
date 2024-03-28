# index
GET http://localhost:8080/

HTTP 200
[Asserts]
body contains "<p>Hello world!"

# routing
GET http://localhost:8080/routing

HTTP 200
[Asserts]
body contains "routing"

GET http://localhost:8080/routing/file

HTTP 200
[Asserts]
body contains "<p>hello!"

# calling another template
GET http://localhost:8080/routing/visible

HTTP 200
[Asserts]
body contains "<p>You can't see me"

# reading files from context, define a handler inline
GET http://localhost:8080/hello

HTTP 200
[Asserts]
body contains "world"

# nonexistent file should not be routable
GET http://localhost:8080/routing/_hidden.html

HTTP 404


GET http://localhost:8080/routing/_hidden

HTTP 404


# Hidden file should not be routable
GET http://localhost:8080/routing/.hidden.html

HTTP 404


GET http://localhost:8080/routing/.hidden

HTTP 404

