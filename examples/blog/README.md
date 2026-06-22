# blog

A tiny blog that demonstrates optimal static asset serving in xtemplate:
content-hash cache-busting plus Subresource Integrity (SRI).

```
mise run example-blog
```

Then open http://localhost:9003/.

Templates embed a stylesheet's precomputed content hash with
`{{.X.StaticFileHash "/assets/style.css"}}`, rendering:

```html
<link rel="stylesheet" href="/assets/style.css?hash=sha384-..." integrity="sha384-...">
```

Why it matters: the `?hash=` query makes xtemplate serve the asset with a
1-year `immutable` `Cache-Control`, so browsers cache it indefinitely. When the
file changes its hash changes, producing a new URL that bypasses the old cache.
The `integrity` attribute is SRI: the browser refuses to apply the file unless
its bytes match the hash.

## Posts

Posts live as Markdown files with YAML front matter in `posts/*.md`, exposed to
templates through a `Posts` directory provider (see `config.json`). A single
file-routed template, `templates/posts/{slug}.html`, reads `posts/<slug>.md`
and uses `markdown` to pull out the `title`/`date` front matter (`.Meta`) and
render the body (`.Body`) in one pass.

The home page doesn't re-read every post on each request. Instead
`templates/.init.html` defines an `INIT` template that runs once at startup,
reads every post's front matter, and caches it in an in-memory SQLite table;
the index then lists posts with a single `SELECT`. Because `config.json`
watches the `posts` directory (`watch_dirs`), adding, editing, or deleting a
post reloads the instance and rebuilds the cache automatically. See the
["Caching post metadata in SQLite"](posts/caching-post-metadata.md) post for
the full walkthrough.
