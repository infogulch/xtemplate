---
status: accepted
---

# Reload rebuilds a whole immutable instance and swaps it atomically

A loaded template root is an immutable instance. The server holds the current one in an `atomic.Pointer` and reload builds a full new instance from config, then CAS-swaps it. Request handling does a lock-free load and never sees a half-configured template instance.

## Considered options

- **Hot-patch only changed templates on the live instance.** Rejected: needs locking on the hot path; partial updates can leave the global namespace inconsistent (cross-template refs mid-change); requires deleting template definitions from the html/templates object; requires deleting routes from the ServeMux (not supported).
- **Parse from disk on every request.** Rejected: defeats the cached-instance latency goal.

## Consequences

- Reload cost scales with the whole root (off the request path: deploy / dev watch / git poll).
- Failed build → no swap; old instance keeps serving.
- Old and new briefly coexist; after swap the old `Config.Ctx` is cancelled (SSE live-reload waits on that). Reloads are mutex-serialized; only the pointer swap is visible to handlers.
