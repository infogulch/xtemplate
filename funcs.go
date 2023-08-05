package xtemplate

import (
	"bytes"
	"fmt"
	"html/template"
	"reflect"
	"strconv"
	"strings"
	"time"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dustin/go-humanize"
	"github.com/google/uuid"
	"github.com/segmentio/ksuid"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

var funcLibrary template.FuncMap = template.FuncMap{
	"stripHTML":        funcStripHTML,
	"markdown":         funcMarkdown,
	"splitFrontMatter": funcSplitFrontMatter,
	"httpError":        funcHTTPError,
	"humanize":         funcHumanize,
	"trustHtml":        funcTrustHtml,
	"trustAttr":        funcTrustAttr,
	"trustJS":          funcTrustJS,
	"trustJSStr":       funcTrustJSStr,
	"trustSrcSet":      funcTrustSrcSet,
	"uuid":             funcUuid,
	"idx":              funcIdx,
	"ksuid":            funcKsuid,
	"try":              funcTry,
}

// funcStripHTML returns s without HTML tags. It is fairly naive
// but works with most valid HTML inputs.
func funcStripHTML(s string) string {
	var buf bytes.Buffer
	var inTag, inQuotes bool
	var tagStart int
	for i, ch := range s {
		if inTag {
			if ch == '>' && !inQuotes {
				inTag = false
			} else if ch == '<' && !inQuotes {
				// false start
				buf.WriteString(s[tagStart:i])
				tagStart = i
			} else if ch == '"' {
				inQuotes = !inQuotes
			}
			continue
		}
		if ch == '<' {
			inTag = true
			tagStart = i
			continue
		}
		buf.WriteRune(ch)
	}
	if inTag {
		// false start
		buf.WriteString(s[tagStart:])
	}
	return buf.String()
}

// funcMarkdown renders the markdown body as HTML. The resulting
// HTML is NOT escaped so that it can be rendered as HTML.
func funcMarkdown(input any) (string, error) {
	inputStr := caddy.ToString(input)

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			highlighting.NewHighlighting(
				highlighting.WithFormatOptions(
					chromahtml.WithClasses(true),
				),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			gmhtml.WithUnsafe(), // TODO: this is not awesome, maybe should be configurable?
		),
	)

	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	err := md.Convert([]byte(inputStr), buf)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// splitFrontMatter parses front matter out from the beginning of input,
// and returns the separated key-value pairs and the body/content. input
// must be a "stringy" value.
func funcSplitFrontMatter(input any) (parsedMarkdownDoc, error) {
	meta, body, err := extractFrontMatter(caddy.ToString(input))
	if err != nil {
		return parsedMarkdownDoc{}, err
	}
	return parsedMarkdownDoc{Meta: meta, Body: body}, nil
}

// funcHTTPError returns a structured HTTP handler error. EXPERIMENTAL; SUBJECT TO CHANGE.
// Example usage: `{{if not (fileExists $includeFile)}}{{httpError 404}}{{end}}`
func funcHTTPError(statusCode int) (bool, error) {
	return false, caddyhttp.Error(statusCode, nil)
}

// funcTrustHtml marks the string s as safe and does not escape its contents in
// html node context.
func funcTrustHtml(s string) template.HTML {
	return template.HTML(s)
}

// funcTrustHtml marks the string s as safe and does not escape its contents in
// html attribute context. For example, ` dir="ltr"`.
func funcTrustAttr(s string) template.HTMLAttr {
	return template.HTMLAttr(s)
}

// funcTrustJS marks the string s as safe and does not escape its contents in
// script tag context.
func funcTrustJS(s string) template.JS {
	return template.JS(s)
}

// funcTrustJSStr marks the string s as safe and does not escape its contents in
// script tag context.
func funcTrustJSStr(s string) template.JSStr {
	return template.JSStr(s)
}

// funcTrustSrcSet marks the string s as safe and does not escape its contents in
// script tag context.
func funcTrustSrcSet(s string) template.Srcset {
	return template.Srcset(s)
}

func funcUuid() uuid.UUID {
	return uuid.New()
}

func funcIdx(idx int, arr any) any {
	return reflect.ValueOf(arr).Index(idx).Interface()
}

func funcKsuid() ksuid.KSUID {
	return ksuid.New()
}

// funcHumanize transforms size and time inputs to a human readable format.
//
// Size inputs are expected to be integers, and are formatted as a
// byte size, such as "83 MB".
//
// Time inputs are parsed using the given layout (default layout is RFC1123Z)
// and are formatted as a relative time, such as "2 weeks ago".
// See https://pkg.go.dev/time#pkg-constants for time layout docs.
func funcHumanize(formatType, data string) (string, error) {
	// The format type can optionally be followed
	// by a colon to provide arguments for the format
	parts := strings.Split(formatType, ":")

	switch parts[0] {
	case "size":
		dataint, dataerr := strconv.ParseUint(data, 10, 64)
		if dataerr != nil {
			return "", fmt.Errorf("humanize: size cannot be parsed: %s", dataerr.Error())
		}
		return humanize.Bytes(dataint), nil

	case "time":
		timelayout := time.RFC1123Z
		if len(parts) > 1 {
			timelayout = parts[1]
		}

		dataint, dataerr := time.Parse(timelayout, data)
		if dataerr != nil {
			return "", fmt.Errorf("humanize: time cannot be parsed: %s", dataerr.Error())
		}
		return humanize.Time(dataint), nil
	}

	return "", fmt.Errorf("no know function was given")
}

func funcTry(fn any, args ...any) (*result, error) {
	r := reflect.ValueOf(fn)
	if r.Kind() != reflect.Func {
		return nil, fmt.Errorf("not a function")
	}
	n := r.Type().NumOut()
	if n != 1 && n != 2 {
		return nil, fmt.Errorf("cannot call func that has %d outputs", n)
	} else if !r.Type().Out(n - 1).AssignableTo(reflect.TypeOf((*error)(nil)).Elem()) {
		return nil, fmt.Errorf("cannot call func whose last arg is not error")
	}
	reflectArgs := []reflect.Value{}
	for _, a := range args {
		reflectArgs = append(reflectArgs, reflect.ValueOf(a))
	}
	var out []reflect.Value
	if r.Type().IsVariadic() {
		out = r.CallSlice(reflectArgs)
	} else {
		out = r.Call(reflectArgs)
	}
	var err error
	var value any
	ierr := out[n-1].Interface()
	if ierr != nil {
		err = ierr.(error)
	}
	if n > 1 {
		value = out[0].Interface()
	}
	return &result{
		Value: value,
		Error: err,
	}, nil
}

type result struct {
	Value any
	Error error
}

func (r *result) OK() bool {
	return r.Error == nil
}

// Skeleton versions of the built-in functions in templates. This is needed to
// make text/template/parse.Parse parse correctly because the number of
// arguments is checked at parse time, but they are never not called and the
// argument types are not checked, just their number.
var buliltinsSkeleton template.FuncMap = template.FuncMap{
	"and":      func(any, ...any) any { return nil },
	"call":     func(any, ...any) (any, error) { return nil, nil },
	"html":     template.HTMLEscaper,
	"index":    func(any, ...any) (any, error) { return nil, nil },
	"slice":    func(any, ...any) (any, error) { return nil, nil },
	"js":       template.JSEscaper,
	"len":      func(any) (int, error) { return 0, nil },
	"not":      func(any) bool { return false },
	"or":       func(any, ...any) any { return nil },
	"print":    fmt.Sprint,
	"printf":   fmt.Sprintf,
	"println":  fmt.Sprintln,
	"urlquery": template.URLQueryEscaper,

	// Comparisons
	"eq": func(any, ...any) (bool, error) { return false, nil }, // ==
	"ge": func(any, ...any) (bool, error) { return false, nil }, // >=
	"gt": func(any, ...any) (bool, error) { return false, nil }, // >
	"le": func(any, ...any) (bool, error) { return false, nil }, // <=
	"lt": func(any, ...any) (bool, error) { return false, nil }, // <
	"ne": func(any, ...any) (bool, error) { return false, nil }, // !=
}
