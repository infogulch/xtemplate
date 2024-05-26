GET http://localhost:8080/db/manual

HTTP 200
[Asserts]
body contains "Run a manual migration"
body contains "manual.1.sql (1)"


POST http://localhost:8080/db/run
[FormParams]
id: 1

HTTP 200
[Asserts]
body contains "Applied migration 1."
