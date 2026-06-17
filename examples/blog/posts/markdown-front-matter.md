---
title: Markdown posts with front matter
date: "2024-02-03"
summary: How a single template turns a .md file into a finished page.
---

A post is just a Markdown file that starts with a fenced block of metadata —
its *front matter*:

```md
---
title: Markdown posts with front matter
date: "2024-02-03"
summary: How a single template turns a .md file into a finished page.
---

Your post body goes here...
```

xtemplate ships the two functions that make this work:

- `splitFrontMatter` parses that opening block, returning the metadata as
  `.Meta` and everything after it as `.Body`. YAML (`---`), TOML (`+++`), and
  JSON front matter are all understood.
- `markdown` renders the body to HTML (CommonMark, GitHub-flavored, with syntax
  highlighting).

The whole blog runs on one template, `posts/{slug}.html`, which reads
`posts/<slug>.md` through the `Posts` directory provider, splits the front
matter for the `<title>` and heading, and renders the body:

```html
{{$post := splitFrontMatter (.Posts.Read (print (.Req.PathValue "slug") ".md"))}}
<h1>{{$post.Meta.title}}</h1>
{{markdown $post.Body}}
```

The home page reuses the same metadata: it lists the directory with
`.Posts.ReadDir` and reads each file's `title` and `summary` for the index.

[Back home](/)
