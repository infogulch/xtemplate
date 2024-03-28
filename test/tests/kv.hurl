# get kv
GET http://localhost:8080/kv

HTTP 200
[Asserts]
body contains "a: 1"
