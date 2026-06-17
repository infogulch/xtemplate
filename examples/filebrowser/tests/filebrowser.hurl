# listing the FS root
GET http://localhost:8080/

HTTP 200
[Asserts]
body contains "readme.txt"
body contains "notes.md"

# viewing a file's contents
GET http://localhost:8080/view/readme.txt

HTTP 200
[Asserts]
body contains "Welcome to the xtemplate filebrowser example."

# missing file
GET http://localhost:8080/view/nope.txt

HTTP 404

# directory listing
GET http://localhost:8080/docs/

HTTP 200
[Asserts]
body contains "hello.txt"

# directory without trailing slash redirects
GET http://localhost:8080/docs

HTTP 307

# viewing a file in a directory
GET http://localhost:8080/view/docs/hello.txt

HTTP 200
