# no Accept-Encoding should return identity
GET http://localhost:8080/assets/file.txt

HTTP 200
Content-Type: text/plain; charset=utf-8
Content-Encoding: identity
[Asserts]
body == "testing"


# accept gzip should return gzip
GET http://localhost:8080/assets/file.txt
Accept-Encoding: gzip

HTTP 200
Content-Type: text/plain; charset=utf-8
Content-Encoding: gzip
[Asserts]
body == "testing"


# accept gzip or identity should return identity
GET http://localhost:8080/assets/file.txt
Accept-Encoding: gzip, identity

HTTP 200
Content-Type: text/plain; charset=utf-8
Content-Encoding: identity


# accept gzip or identity with 0.09 pref to gzip should return identity
GET http://localhost:8080/assets/file.txt
Accept-Encoding: gzip;q=0.5, identity;q=0.41

HTTP 200
Content-Type: text/plain; charset=utf-8
Content-Encoding: identity


# accept gzip or identity with 0.11 pref to gzip should return gzip
GET http://localhost:8080/assets/file.txt
Accept-Encoding: gzip;q=0.5, identity;q=0.39

HTTP 200
Content-Type: text/plain; charset=utf-8
Content-Encoding: gzip


# accept non existent encoding should return identity
GET http://localhost:8080/assets/file.txt
Accept-Encoding: br

HTTP 200
Content-Type: text/plain; charset=utf-8
Content-Encoding: identity


# Empty file
GET http://localhost:8080/assets/empty.txt

HTTP 200
Content-Type: text/plain; charset=utf-8
Content-Encoding: identity
[Asserts]
body == ""


# CSS file
GET http://localhost:8080/assets/reset.css
Accept-Encoding: gzip

HTTP 200
Content-Type: text/css; charset=utf-8
Content-Encoding: gzip


# CSS file
GET http://localhost:8080/assets/reset.css
Accept-Encoding: gzip, br

HTTP 200
Content-Type: text/css; charset=utf-8
Content-Encoding: br
Etag: "sha384-5rcfZgbOPW7qvI7_bo9eNa8hclwmmmzNeyvDzZlqI6vAzNwzbmi7PTS4uA15-fJj"


# CSS file
GET http://localhost:8080/assets/reset.css?hash=sha384-5rcfZgbOPW7qvI7_bo9eNa8hclwmmmzNeyvDzZlqI6vAzNwzbmi7PTS4uA15-fJj
Accept-Encoding: gzip

HTTP 200
Content-Type: text/css; charset=utf-8
Content-Encoding: gzip
Etag: "sha384-5rcfZgbOPW7qvI7_bo9eNa8hclwmmmzNeyvDzZlqI6vAzNwzbmi7PTS4uA15-fJj"
Cache-Control: public, max-age=31536000, immutable


# CSS file
GET http://localhost:8080/assets/reset.css
If-None-Match: "sha384-5rcfZgbOPW7qvI7_bo9eNa8hclwmmmzNeyvDzZlqI6vAzNwzbmi7PTS4uA15-fJj"

HTTP 304
Etag: "sha384-5rcfZgbOPW7qvI7_bo9eNa8hclwmmmzNeyvDzZlqI6vAzNwzbmi7PTS4uA15-fJj"

# Standalone gzip file should not be accessible without its extension
GET http://localhost:8080/assets/standalone

HTTP 404

# Standalone gzip file should only be accessible by it's full path
GET http://localhost:8080/assets/standalone.gz

HTTP 200
Content-Encoding: identity

# check that index includes integrity attribute and hash parameter
GET http://localhost:8080/

HTTP 200
[Asserts]
xpath "string(//link[@rel='stylesheet']/@integrity)" startsWith "sha384-5rcfZ"
xpath "string(//link[@rel='stylesheet']/@href)" contains "?hash=sha384-5rcfZ"

# get favicon
GET http://localhost:8080/favicon.ico

HTTP 200
