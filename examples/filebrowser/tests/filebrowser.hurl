# listing the FS root
GET http://localhost:8080/

HTTP 200
[Asserts]
body contains "readme.txt"
body contains "notes.md"
body contains "multipart/form-data"

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

# multipart upload into the FS root (writable fs → ReceiveFiles)
POST http://localhost:8080/upload
[Multipart]
file: file,upload-sample.txt; text/plain

HTTP 303
[Captures]
upload_loc: header "Location"
[Asserts]
header "Location" matches "^/[^/]+/$"

# listing the per-request upload dir shows the stored file
GET http://localhost:8080{{upload_loc}}

HTTP 200
[Asserts]
body contains "00.txt"
body contains "upload.json"

# view the stored upload contents
GET http://localhost:8080/view{{upload_loc}}00.txt

HTTP 200
[Asserts]
body contains "hello from filebrowser upload test"

# upload into a subdirectory via ?dir=
POST http://localhost:8080/upload?dir=docs/
[Multipart]
file: file,upload-sample.txt; text/plain

HTTP 303
[Captures]
docs_upload_loc: header "Location"
[Asserts]
header "Location" matches "^/docs/[^/]+/$"

GET http://localhost:8080{{docs_upload_loc}}

HTTP 200
[Asserts]
body contains "00.txt"
