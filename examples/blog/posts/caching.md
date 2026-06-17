---
title: Why hashed asset URLs matter
date: "2024-01-22"
summary: Content-hash cache-busting plus Subresource Integrity, in one line.
---

Every page on this blog links its stylesheet like this:

```html
<link rel="stylesheet" href="/assets/style.css?hash=sha384-..." integrity="sha384-...">
```

Both values come from a single template call,
`{{.X.StaticFileHash "/assets/style.css"}}`, which embeds the file's
precomputed content hash.

The `?hash=` query does two jobs:

- It makes xtemplate serve the asset with a one-year `immutable`
  `Cache-Control`, so browsers cache it indefinitely.
- When the file changes its hash changes, producing a brand-new URL that
  sidesteps the old cache. No manual versioning, no stale CSS.

The `integrity` attribute is **Subresource Integrity** (SRI): the browser
refuses to apply the file unless its bytes match the hash, so a tampered or
corrupted asset is rejected outright.

[Back home](/)
