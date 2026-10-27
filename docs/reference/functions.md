# Template functions

Template functions are callable from any template as `{{name args…}}`. They come from FuncMaps fixed when the instance is built. They do not see the current request; request-scoped behavior belongs on the [dot context](dot-context.md) via dot fields.

Add FuncMaps with `Config.FuncMaps` or `xtemplate.WithFuncMaps(...)`.

Three sets are present by default: xtemplate, Sprout (with Sprig-compatible names), and Go’s built-ins (stdlib).

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

## Sprout functions

[Sprout](https://docs.atom.codes/sprout) supplies string, math, date, list, dict, encoding, path, slice, and regex helpers. xtemplate registers them through Sprout's [`sprigin`](https://pkg.go.dev/github.com/go-sprout/sprout/sprigin) compatibility layer (the same registries Sprig exposed, plus Sprout's aliases), so existing templates keep working.

Examples: `upper`, `default`, `dict`, `list`, `date`, `toJson`, `fromJson` (decode a raw body from [`.Req.ReadBody`](dot-context.md#request-details-in-req)). Full catalog: [Sprout registries](https://docs.atom.codes/sprout/registries).

Sprig names and Sprout names both resolve. `upper` is `toUpper`, `atoi` and `int` are `toInt`, `toJson` / `fromJson` are unchanged, and the same pattern holds for string, slice, math, and date helpers (`ago` / `dateAgo`, `b64enc` / `base64Encode`, …). `get`, `set`, `unset`, `hasKey`, `pick`, `omit`, `append`, `prepend`, `slice`, and `without` accept the Sprig argument order (`unset $dict "key"`, `slice $list 1`) and the Sprout pipeline order (`$dict | unset "key"`, `$list | slice 1`).

```html
{{.Req.FormValue "name" | default "World" | upper}}
{{dict "name" "Ada" | toJson}}
{{`{"n":1}` | fromJson}}
```

Differences from Sprig, inherited from `sprigin`:

- Functions that panicked in Sprig (deep copy, some list and random helpers) no longer crash the process.
- A helper whose name does not start with `must` does not abort the template when it returns an error. `mustFromJson`, `mustToJson`, and the other `must*` forms still do. Use those when bad input should fail the request. Sprig's `fail` is in this non-aborting group; use xtemplate's `failf` to stop rendering.
- A legacy Sprig name or the old argument order logs a deprecation warning on the instance logger. The call still succeeds.
- `get` on a missing key still returns `""` (same as Sprig). `.Vars.Get` returns `nil`.

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
