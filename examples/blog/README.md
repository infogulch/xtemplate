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
