---
title: Hello, world
date: "2024-01-15"
summary: A blog where every post is a Markdown file rendered on the fly.
---

This is the first post.

Each post lives as a plain Markdown file in `posts/`. xtemplate reads the file,
splits off its [front matter](/posts/markdown-front-matter), and renders the
body to HTML at request time with the built-in `markdown` function - no build
step, no database.

Adding a post is as easy as dropping a new `.md` file in the folder. The home
page picks it up automatically by listing the directory.

[Back home](/)
