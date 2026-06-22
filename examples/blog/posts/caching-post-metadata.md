---
title: Caching post metadata in SQLite
date: "2024-02-10"
summary: An INIT template fills a cache at startup; a directory watch keeps it fresh.
---

The home page lists every post. The obvious way to build that list is to read
each `.md` file, split its front matter, and collect the titles on every
request. That works, but it re-reads and re-parses every post each time someone
loads the index — fine for three posts, wasteful for three hundred.

Instead this blog builds the list **once** and serves it from a cache.

## Fill the cache at startup

Any template whose name starts with `INIT ` runs a single time when xtemplate
loads, with full access to the dot. `templates/.init.html` uses that to read
every post's front matter and store it in an in-memory SQLite table:

```html
{{define "INIT cache-posts"}}
{{$_ := .DB.Exec `CREATE TABLE IF NOT EXISTS posts (slug TEXT PRIMARY KEY, title TEXT, date TEXT, summary TEXT)`}}
{{$_ = .DB.Exec `DELETE FROM posts`}}
{{range .Posts.ReadDir `.`}}
{{$post := markdown ($.Posts.Read .Name)}}
{{$_ = $.DB.Exec `INSERT INTO posts (slug, title, date, summary) VALUES (?, ?, ?, ?)` (trimSuffix `.md` .Name) $post.Meta.title $post.Meta.date $post.Meta.summary}}
{{end}}
{{end}}
```

Now the index is one query, no file I/O:

```html
{{range .DB.QueryRows `SELECT slug, title, summary FROM posts ORDER BY date DESC`}}
<li><a href="/posts/{{.slug}}">{{.title}}</a> — {{.summary}}</li>
{{end}}
```

The database lives entirely in memory (`"connstr": ":memory:"` in
`config.json`). A bare in-memory SQLite database is private to a single
connection, so `"max_open_conns": 1` pins the pool to one connection — that is
what lets the `INIT` writes and the index reads see the same table.

## Keep it fresh

A cache is only useful if it stays correct. The usual hard part — knowing when
to invalidate — disappears here because the cache is *derived state rebuilt on
every reload*. `config.json` watches the posts directory:

```json
"watch_dirs": ["posts"]
```

Add, edit, or delete a post and xtemplate reloads the instance, which re-runs
the `INIT` template and rebuilds the table from scratch. The `DELETE FROM posts`
keeps that rebuild idempotent. No invalidation logic, no stale rows — just
"throw it away and recompute," which is cheap for a blog's worth of files.

The single-post page still renders on demand: it reads and renders only the one
file being requested. Caching the list doesn't mean caching everything.

[Back home](/)
