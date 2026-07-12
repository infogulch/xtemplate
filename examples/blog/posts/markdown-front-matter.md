---
title: Markdown posts with front matter
date: "2024-02-03"
summary: How a single template turns a .md file into a finished page.
---

A post is just a Markdown file that starts with a fenced block of metadata -
its *front matter*:

```md
---
title: Markdown posts with front matter
date: "2024-02-03"
summary: How a single template turns a .md file into a finished page.
---

Your post body goes here...
```

The `markdown` function does it all in one pass: it parses that opening block
and renders the rest, returning the metadata as `.Meta` and the rendered HTML
as `.Body`. YAML (`---`) and TOML (`+++`) front matter are understood, and the
body is rendered as CommonMark, GitHub-flavored, with syntax highlighting. Its
input can be a string or an `io.Reader`.

The whole blog runs on one template, `posts/{slug}.html`, which reads
`posts/<slug>.md` through the `Posts` directory provider and uses the front
matter for the `<title>` and heading alongside the rendered body:

```html
{{$post := markdown (.Posts.Read (print (.Req.PathValue "slug") ".md"))}}
<h1>{{$post.Meta.title}}</h1>
{{$post.Body}}
```

The home page reuses the same metadata: it lists the directory with
`.Posts.ReadDir` and reads each file's `title` and `summary` for the index.

[Back home](/)
