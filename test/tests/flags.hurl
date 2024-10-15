# get kv
GET http://localhost:8080/flags

HTTP 200
[Asserts]
body contains "a: 1"
