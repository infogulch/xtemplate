# Template functions

Template functions are callable from any template as `{{name args…}}`. They come from FuncMaps fixed when the instance is built. They do not see the current request; request-scoped behavior belongs on the [dot context](dot-context.md) via dot fields.

Add FuncMaps with `Config.FuncMaps` or `xtemplate.WithFuncMaps(...)`.

Three sets are present by default: xtemplate, Sprig, and Go’s built-ins (stdlib).

## xtemplate functions

| Name | Summary |
|---|---|
| `markdown` | Render CommonMark/GFM (Goldmark) to HTML; optional front matter → `.Meta` / `.Body` |
| `sanitizeHtml` | Sanitize HTML with a named BlueMonday policy (`strict`, `ugc`, `externalugc`, …) |
| `humanize` | Human-readable `size` (bytes) or `time` (relative) strings |
| `try` | Call a fallible func/method and get a result object instead of aborting |
| `return` | Early-return successfully from the template |
| `failf` | Fail execution with a formatted error (not an early return) |
| `trustHtml` / `trustAttr` / `trustJS` / `trustJSStr` / `trustSrcSet` | Mark a string as trusted for a given context (escapes disabled) |
| `idx` | Index into a slice/array/string with index first (pipeline-friendly) |

Godoc entry points (same package): [`FuncMarkdown`](https://pkg.go.dev/github.com/infogulch/xtemplate#FuncMarkdown), [`FuncSanitizeHtml`](https://pkg.go.dev/github.com/infogulch/xtemplate#FuncSanitizeHtml), [`FuncHumanize`](https://pkg.go.dev/github.com/infogulch/xtemplate#FuncHumanize), [`FuncTry`](https://pkg.go.dev/github.com/infogulch/xtemplate#FuncTry), and siblings named `Func…`.

### Examples

```html
{{/* Markdown with optional front matter */}}
{{with markdown (.FS.Read "posts/hello.md")}}
  <h1>{{index .Meta "title"}}</h1>
  {{.Body}}
{{end}}

{{/* Sanitize untrusted HTML */}}
{{sanitizeHtml "ugc" .userHTML}}

{{/* Human-readable size */}}
{{humanize "size" "2048000"}}

{{/* Early success exit */}}
{{if not .ok}}{{return}}{{end}}

{{/* Hard failure */}}
{{if eq (.Req.FormValue "name") ""}}{{failf "name is required"}}{{end}}
```

Register extra BlueMonday policies or Goldmark configs globally with `AddBlueMondayPolicy` and `AddMarkdownConfig` if needed (see godoc).

## Sprig functions

[Sprig](https://masterminds.github.io/sprig/) supplies string, math, date, list, dict, encoding, and path helpers. Examples: `upper`, `default`, `dict`, `list`, `date`, `toJson`. Full catalog: [Sprig Function Documentation](https://masterminds.github.io/sprig/).

```html
{{.Req.FormValue "name" | default "World" | upper}}
```

## Go built-in functions

Go's template builtins: `and`, `or`, `not`, `eq`, `lt`, `index`, `printf`, `len`, `slice`, `urlquery`, and the rest. See [text/template - Functions](https://pkg.go.dev/text/template#hdr-Functions).

## Dot fields vs template functions

| | Template functions | Dot fields (from dot providers) |
|---|---|---|
| Call style | `{{markdown x}}` | `{{.DB.QueryRow …}}` |
| Request access | No | Yes |
| Configuration | `FuncMaps` at load | Provider config / `WithProvider` / registry |
| Best for | Pure transforms | I/O and response control |

See also [Template semantics](template-semantics.md).
