# Draft: docs app

Render the RFD series (`docs/rfd/*.md`) as a small website using xtemplate, so the project's design records are publicly readable and xtemplate dogfoods itself. Target deployment: `xtemplate.dev/docs`.

## Goal

An xtemplate instance that serves `docs/` as HTML: an index page listing all RFDs (with title, tags, supersession status), and a page per RFD rendering its markdown body with frontmatter parsed for metadata. The leading candidate for the app's home is `examples/` (following the existing `examples/blog`, `examples/contacts`, etc. convention), which would double as a user-facing example of xtemplate.

## Motivation

- **Dogfooding closed loop.** xtemplate renders its own design records. The strongest proof a templating server works is using it for real, visible content.
- **Public readability.** RFDs are frozen design records; they're valuable to readers (contributors, users evaluating the project) only if they're renderable, not just raw markdown in a repo.
- **Example value.** A docs site is a common real-world use case for a templating engine; as an example app it would join the existing `examples/blog` etc. as something users can study.

## Rough shape (open questions, not decisions)

- **Where the app lives.** `examples/docs/` is the obvious candidate (matches `examples/blog` etc.). Open: does it need its own `cmd/` binary, or does it run via `app.Main` like other examples? Check how existing examples are structured.
- **Dot providers needed.** A `Dir` (or `fs`) provider pointing at `docs/rfd/` to read RFD files and list the directory. Probably no `db` or `flags` needed. The `markdown` function (already in `funcs.go`) renders bodies and parses frontmatter into `.Meta`.
- **Routing.** xtemplate maps URL paths to template files. `/docs/` → index, `/docs/000N-slug` → the RFD page. Need to figure out how the RFD's `000N-slug.md` maps to a URL path — does the request path include the `.md` extension, or is it stripped? How does the index template enumerate the directory (xtemplate has no `files` function; directory listing comes from a `Dir` dot field's `ReadDir` method)?
- **Index page.** `docs/index.md` is currently a manual list. A dynamic index would call `ReadDir` on the `docs/rfd` directory, per-RFD parse frontmatter for title/tags/supersession, and render links. Whether to keep it manual or make it dynamic is an open question — dynamic is more work but stays correct as RFDs are added.
- **Supersession display.** An RFD with an `## Addendum: Superseded by 000N` addendum should show as superseded in the index. The addendum is in the body, not frontmatter (per 0001), so detecting supersession requires parsing the body — not just frontmatter. Open: is grep-ing the rendered body acceptable, or should the index just link to each RFD and let the reader see the addendum on the page?
- **Frontmatter rendering.** The `markdown` function already returns `.Meta` (frontmatter) and `.Body` (rendered HTML). Tags, `supersedes`, and provenance fields are available to templates via `.Meta.tags`, `.Meta.supersedes`, etc. No new parsing needed.
- **Deployment.** `xtemplate.dev/docs` — is this served by the same binary as the rest of xtemplate.dev, or a separate deployment? Out of scope for this draft until the rest of xtemplate.dev's hosting is understood.

## Open questions

- How do existing examples (`examples/blog`, `examples/filebrowser`) handle directory listing and routing? Study before designing.
- Should `index.md` become a dynamic xtemplate template, or stay manual with the app rendering only individual RFDs?
- Is there a standard xtemplate pattern for "render every markdown file in a directory as a page"?
