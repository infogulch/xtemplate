# DB transaction semantics. The sqlite file is fresh each run, and the
# "INIT db-setup" template creates an empty `items` table at startup.

# index page renders
GET http://localhost:8080/db/

HTTP 200
[Asserts]
body contains "user_version"

# table exists and starts empty
GET http://localhost:8080/db/count

HTTP 200
[Asserts]
body == "0"

# a successful insert commits
POST http://localhost:8080/db/items
[FormParams]
name: alice

HTTP 200

GET http://localhost:8080/db/count

HTTP 200
[Asserts]
body == "1"

# a failing template rolls back its insert
POST http://localhost:8080/db/items-fail
[FormParams]
name: bob

HTTP 500

# the rolled-back insert did not persist
GET http://localhost:8080/db/count

HTTP 200
[Asserts]
body == "1"
