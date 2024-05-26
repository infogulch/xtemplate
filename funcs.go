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
	"github.com/dustin/go-humanize"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

var xtemplateFuncs template.FuncMap = template.FuncMap{
	"sanitizeHtml":     FuncSanitizeHtml,
	"markdown":         FuncMarkdown,
	"splitFrontMatter": FuncSplitFrontMatter,
	"return":           FuncReturn,
	"humanize":         FuncHumanize,
	"trustHtml":        FuncTrustHtml,
	"trustAttr":        FuncTrustAttr,
	"trustJS":          FuncTrustJS,
	"trustJSStr":       FuncTrustJSStr,
	"trustSrcSet":      FuncTrustSrcSet,
	"idx":              FuncIdx,
	"try":              FuncTry,
}

// blueMondayPolicies is the map of names of bluemonday policies available to
// templates.
var blueMondayPolicies map[string]*bluemonday.Policy = map[string]*bluemonday.Policy{
	"strict": bluemonday.StrictPolicy(),
	"ugc":    bluemonday.UGCPolicy(),
	"externalugc": bluemonday.UGCPolicy().
		AddTargetBlankToFullyQualifiedLinks(true).
		AllowRelativeURLs(false),
}

// AddBlueMondayPolicy adds a bluemonday policy to the global policy list available to all
// xtemplate instances.
func AddBlueMondayPolicy(name string, policy *bluemonday.Policy) {
	if old, ok := blueMondayPolicies[name]; ok {
		panic(fmt.Sprintf("bluemonday policy with name %s already exists: %v", name, old))
	}
	blueMondayPolicies[name] = policy
}

// sanitizeHtml Uses the BlueMonday library to sanitize strings with html content.
// First parameter is the name of the chosen sanitization policy.
func FuncSanitizeHtml(policyName string, html string) (template.HTML, error) {
	policy, ok := blueMondayPolicies[policyName]
	if !ok {
		return "", fmt.Errorf("failed to find policy name '%s'", policyName)
	}
	return template.HTML(policy.Sanitize(html)), nil
}

var mdOpts = []goldmark.Option{
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
}

var markdownConfigs map[string]goldmark.Markdown = map[string]goldmark.Markdown{
	"default": goldmark.New(mdOpts...),
	"unsafe": goldmark.New(append(mdOpts,
		goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
	)...),
}

// AddMarkdownConifg adds a custom markdown configuration to xtemplate's
// markdown config map, available to all xtemplate instances.
func AddMarkdownConifg(name string, md goldmark.Markdown) {
	if old, ok := markdownConfigs[name]; ok {
		panic(fmt.Sprintf("markdown policy with name %s already exists: %v", name, old))
	}
	markdownConfigs[name] = md
}

// markdown renders the given Markdown text as HTML and returns it. This uses
// the Goldmark library, which is CommonMark compliant. If an alternative
// markdown policy is not named, it uses the default policy which has these
// extensions enabled: Github Flavored Markdown, Footnote, and syntax
// highlighting provided by Chroma.
func FuncMarkdown(input string, configName ...string) (template.HTML, error) {
	config := "default"
	switch len(configName) {
	case 0:
	case 1:
		config = configName[0]
	default:
		return "", fmt.Errorf("too many configName arguments provided: %v", configName)
	}
	md, ok := markdownConfigs[config]
	if !ok {
		return "", fmt.Errorf("unknown markdown config name: %s", config)
	}

	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	err := md.Convert([]byte(input), buf)
	if err != nil {
		return "", err
	}

	return template.HTML(buf.String()), nil
}

// splitFrontMatter parses front matter out from the beginning of input,
// and returns the separated key-value pairs and the body/content. input
// must be a "stringy" value.
func FuncSplitFrontMatter(input string) (parsedMarkdownDoc, error) {
	meta, body, err := extractFrontMatter(input)
	if err != nil {
		return parsedMarkdownDoc{}, err
	}
	return parsedMarkdownDoc{Meta: meta, Body: body}, nil
}

// return causes the template to exit early with a success status.
func FuncReturn() (string, error) {
	return "", ReturnError{}
}

// trustHtml marks the string s as safe and does not escape its contents in
// html node context.
func FuncTrustHtml(s string) template.HTML {
	return template.HTML(s)
}

// trustAttr marks the string s as safe and does not escape its contents in
// html attribute context.
func FuncTrustAttr(s string) template.HTMLAttr {
	return template.HTMLAttr(s)
}

// trustJS marks the string s as safe and does not escape its contents in
// script tag context.
func FuncTrustJS(s string) template.JS {
	return template.JS(s)
}

// trustJSStr marks the string s as safe and does not escape its contents in
// script expression context.
func FuncTrustJSStr(s string) template.JSStr {
	return template.JSStr(s)
}

// trustSrcSet marks the string s as safe and does not escape its contents in
// script tag context.
func FuncTrustSrcSet(s string) template.Srcset {
	return template.Srcset(s)
}

// idx gets an item from a list, similar to the built-in index, but with
// reversed args: index first, then array. This is useful to use index in a
// pipeline, for example:
//
//	{{generate-list | idx 5}}
func FuncIdx(idx int, arr any) any {
	return reflect.ValueOf(arr).Index(idx).Interface()
}

// humanize transforms size and time inputs to a human readable format using
// the go-humanize library.
//
// Call with two parameters: format type and value to format. Supported format
// types are:
//
// "size" which turns an integer amount of bytes into a string like "2.3 MB",
// for example:
//
//	{{humanize "size" "2048000"}}
//
// "time" which turns a time string into a relative time string like "2 weeks
// ago", for example:
//
//	{{humanize "time" "Fri, 05 May 2022 15:04:05 +0200"}}
func FuncHumanize(formatType, data string) (string, error) {
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
			timelayout = formatType[len(parts[0])+1:]
		}

		dataint, dataerr := time.Parse(timelayout, data)
		if dataerr != nil {
			return "", fmt.Errorf("humanize: time cannot be parsed: %s", dataerr.Error())
		}
		return humanize.Time(dataint), nil
	}

	return "", fmt.Errorf("no know function was given")
}

// The try template func accepts a fallible function object and calls it with
// the provided args. If the function and args are valid, try returns the result
// wrapped in a result object that exposes the return value and error to
// templates. Useful if you want to call a function and handle its error in a
// template. If the function value is invalid or the args cannot be used to call
// it then try raises an error that stops template execution.
func FuncTry(fn any, args ...any) (*result, error) {
	if fn == nil {
		return nil, fmt.Errorf("nil func")
	}
	fnv := reflect.ValueOf(fn)
	if fnv.Kind() != reflect.Func {
		if len(args) == 0 {
			return nil, fmt.Errorf("not callable (no method name provided)")
		}
		methodName, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("not callable (non-string method name)")
		}
		method := fnv.MethodByName(methodName)
		if method.IsValid() {
			fnv = method
			args = args[1:]
		} else {
			return nil, fmt.Errorf("not callable (method not found)")
		}
	}
	n := fnv.Type().NumOut()
	if n != 1 && n != 2 {
		return nil, fmt.Errorf("cannot call func that has %d outputs", n)
	} else if !fnv.Type().Out(n - 1).AssignableTo(reflect.TypeOf((*error)(nil)).Elem()) {
		return nil, fmt.Errorf("cannot call func whose last arg is not error")
	}
	reflectArgs := []reflect.Value{}
	for i, a := range args {
		var arg reflect.Value
		if a != nil {
			arg = reflect.ValueOf(a)
		} else {
			arg = reflect.New(fnv.Type().In(i)).Elem()
		}
		reflectArgs = append(reflectArgs, arg)
	}
	out := fnv.Call(reflectArgs)
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
// arguments is checked at parse time, but they are never called and the
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
