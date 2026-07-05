# Contacts example

A minimal CRUD app: file-based routing (`index.html` → `GET /`), custom method
routes (`POST /contacts`, `POST /contacts/{id}/delete`), and the `.DB` sqlite
dot provider. Plain HTML forms with full-page reloads.

```sh
mise run example-contacts
```

Then open http://localhost:9001/.
