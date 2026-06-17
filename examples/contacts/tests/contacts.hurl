# Contacts CRUD smoke test. Fresh sqlite each run, so the list starts empty.

# index page renders
GET http://localhost:8080/

HTTP 200
[Asserts]
body contains "<h1>Contacts</h1>"
body contains "No contacts yet."

# add a contact, then confirm it shows up on the list
POST http://localhost:8080/contacts
[FormParams]
name: Ada Lovelace
phone: 555-0100

HTTP 303
[Asserts]
header "Location" == "/"

# capture the new contact's id from the delete form action
GET http://localhost:8080/

HTTP 200
[Captures]
id: regex "/contacts/(\\d+)/delete"
[Asserts]
body contains "Ada Lovelace"
body contains "555-0100"

# delete it, then confirm it's gone
POST http://localhost:8080/contacts/{{id}}/delete

HTTP 303
[Asserts]
header "Location" == "/"

GET http://localhost:8080/

HTTP 200
[Asserts]
body not contains "Ada Lovelace"
