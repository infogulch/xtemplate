# index renders the stylesheet with SRI integrity + content-hash query
GET http://localhost:8080/

HTTP 200
[Captures]
hash: xpath "string(//link[@rel='stylesheet']/@integrity)"
[Asserts]
xpath "string(//link[@rel='stylesheet']/@integrity)" startsWith "sha384-"
xpath "string(//link[@rel='stylesheet']/@href)" contains "?hash=sha384-"

# the stylesheet is served directly as a static asset
GET http://localhost:8080/assets/style.css

HTTP 200
Content-Type: text/css; charset=utf-8

# requesting it with the content hash gets a 1-year immutable cache
GET http://localhost:8080/assets/style.css?hash={{hash}}

HTTP 200
Cache-Control: public, max-age=31536000, immutable

# a post page renders too
GET http://localhost:8080/posts/hello

HTTP 200
