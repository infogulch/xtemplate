# get kv
GET http://localhost:8080/flags/

HTTP 200
[Asserts]
body contains "a: 1"

# slashless directory path redirects to canonical trailing-slash URL
GET http://localhost:8080/flags

HTTP 301
[Asserts]
header "Location" == "/flags/"
