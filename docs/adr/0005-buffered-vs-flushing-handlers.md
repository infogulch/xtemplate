---
status: accepted
---

# Buffered vs flushing handlers (and mutually exclusive response dots)

Template routes are served by one of two handler kinds. The default is a buffered template handler: execution writes into a memory buffer, then the buffer is flushed to the client only if execution succeeds. A flushing template handler streams output directly to the `ResponseWriter` and is selected by the pseudo-method `SSE` in a define-template name (registered as `GET` on the path, and rejected unless `Accept: text/event-stream`).

Each kind gets its own dot context assembly on the instance: buffered requests get `.Resp` (and not `.Flush`); flushing requests get `.Flush` (and not `.Resp`). Builtin providers `.X` and `.Req`, plus configured core / custom providers, appear on both. Buffering is what makes mid-render status and header changes safe: nothing has been sent yet if the template errors or returns early via `.Resp`. Streaming needs the opposite contract: bytes leave as they are written, so response control is a different type (`.Flush`) rather than a half-working `.Resp` on a live stream.

## Considered options

- **Always stream (no buffer).** Rejected: templates could not reliably set status or headers after producing body bytes, and a mid-render error would leave a truncated HTTP response. That fights the "template is the handler" model for ordinary HTML routes.
- **Always buffer (including SSE).** Rejected: Server-Sent Events and other long-lived streams require incremental delivery and waiting on channels / shutdown without holding the entire response in memory.
- **One unified response field for both kinds.** Rejected: the safe operations diverge (buffer-then-commit vs write-and-flush). Separate `.Resp` / `.Flush` makes misuse a load-time / compile-time absence of the field rather than a subtle runtime bug.

## Consequences

- Authors must use `{{define "SSE /path"}}` (not `GET`) for streaming routes; clients must send `Accept: text/event-stream` or get `406`.
- Templates must not assume `.Resp` and `.Flush` coexist; which field is present is a property of the route’s handler kind.
- Initialization templates run against the buffered dot (they are not streaming responses).
- Memory cost of ordinary pages is proportional to rendered size until commit.
- Only flushing routes have long-lived connections with no write timeout.
