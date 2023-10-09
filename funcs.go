package xtemplate

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/dustin/go-humanize"
	"github.com/microcosm-cc/bluemonday"
	"github.com/segmentio/ksuid"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

var xtemplateFuncs template.FuncMap = template.FuncMap{
	"sanitizeHtml":     funcSanitizeHtml,
	"markdown":         funcMarkdown,
	"splitFrontMatter": funcSplitFrontMatter,
	"abortWithStatus":  funcAbortWithStatus,
	"return":           funcReturn,
	"status":           funcStatus,
	"humanize":         funcHumanize,
	"trustHtml":        funcTrustHtml,
	"trustAttr":        funcTrustAttr,
	"trustJS":          funcTrustJS,
	"trustJSStr":       funcTrustJSStr,
	"trustSrcSet":      funcTrustSrcSet,
	"ksuid":            funcKsuid,
	"idx":              funcIdx,
	"try":              funcTry,
}

var blueMondayPolicies map[string]*bluemonday.Policy = map[string]*bluemonday.Policy{
	"strict": bluemonday.StrictPolicy(),
	"ugc":    bluemonday.UGCPolicy(),
	"externalugc": bluemonday.UGCPolicy().
		AddTargetBlankToFullyQualifiedLinks(true).
		AllowRelativeURLs(false),
}

func funcSanitizeHtml(policyName string, html string) (template.HTML, error) {
	policy, ok := blueMondayPolicies[policyName]
	if !ok {
		return "", fmt.Errorf("failed to find policy name '%s'", policyName)
	}
	return template.HTML(policy.Sanitize(html)), nil
}

// funcMarkdown renders the markdown body as HTML. The resulting
// HTML is NOT escaped so that it can be rendered as HTML.
func funcMarkdown(input string) (string, error) {
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

	err := md.Convert([]byte(input), buf)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// splitFrontMatter parses front matter out from the beginning of input,
// and returns the separated key-value pairs and the body/content. input
// must be a "stringy" value.
func funcSplitFrontMatter(input string) (parsedMarkdownDoc, error) {
	meta, body, err := extractFrontMatter(input)
	if err != nil {
		return parsedMarkdownDoc{}, err
	}
	return parsedMarkdownDoc{Meta: meta, Body: body}, nil
}

// funcReturn causes the template to exit early
func funcReturn() (string, error) {
	return "", &ReturnError{}
}

// funcAbortWithStatus stops rendering the reponse template and immediately returns the status indicated.
// Example usage: `{{if not (fileExists $includeFile)}}{{abortWithStatus 404}}{{end}}`
func funcAbortWithStatus(statusCode int) (struct{}, error) {
	return struct{}{}, NewHandlerError("abort", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
	})
}

// See status.go
var validStatus = map[string]int{
	"Continue":           http.StatusContinue,
	"SwitchingProtocols": http.StatusSwitchingProtocols,
	"Processing":         http.StatusProcessing,
	"EarlyHints":         http.StatusEarlyHints,

	"OK":                   http.StatusOK,
	"Created":              http.StatusCreated,
	"Accepted":             http.StatusAccepted,
	"NonAuthoritativeInfo": http.StatusNonAuthoritativeInfo,
	"NoContent":            http.StatusNoContent,
	"ResetContent":         http.StatusResetContent,
	"PartialContent":       http.StatusPartialContent,
	"MultiStatus":          http.StatusMultiStatus,
	"AlreadyReported":      http.StatusAlreadyReported,
	"IMUsed":               http.StatusIMUsed,

	"MultipleChoices":   http.StatusMultipleChoices,
	"MovedPermanently":  http.StatusMovedPermanently,
	"Found":             http.StatusFound,
	"SeeOther":          http.StatusSeeOther,
	"NotModified":       http.StatusNotModified,
	"UseProxy":          http.StatusUseProxy,
	"TemporaryRedirect": http.StatusTemporaryRedirect,
	"PermanentRedirect": http.StatusPermanentRedirect,

	"BadRequest":                   http.StatusBadRequest,
	"Unauthorized":                 http.StatusUnauthorized,
	"PaymentRequired":              http.StatusPaymentRequired,
	"Forbidden":                    http.StatusForbidden,
	"NotFound":                     http.StatusNotFound,
	"MethodNotAllowed":             http.StatusMethodNotAllowed,
	"NotAcceptable":                http.StatusNotAcceptable,
	"ProxyAuthRequired":            http.StatusProxyAuthRequired,
	"RequestTimeout":               http.StatusRequestTimeout,
	"Conflict":                     http.StatusConflict,
	"Gone":                         http.StatusGone,
	"LengthRequired":               http.StatusLengthRequired,
	"PreconditionFailed":           http.StatusPreconditionFailed,
	"RequestEntityTooLarge":        http.StatusRequestEntityTooLarge,
	"RequestURITooLong":            http.StatusRequestURITooLong,
	"UnsupportedMediaType":         http.StatusUnsupportedMediaType,
	"RequestedRangeNotSatisfiable": http.StatusRequestedRangeNotSatisfiable,
	"ExpectationFailed":            http.StatusExpectationFailed,
	"Teapot":                       http.StatusTeapot,
	"MisdirectedRequest":           http.StatusMisdirectedRequest,
	"UnprocessableEntity":          http.StatusUnprocessableEntity,
	"Locked":                       http.StatusLocked,
	"FailedDependency":             http.StatusFailedDependency,
	"TooEarly":                     http.StatusTooEarly,
	"UpgradeRequired":              http.StatusUpgradeRequired,
	"PreconditionRequired":         http.StatusPreconditionRequired,
	"TooManyRequests":              http.StatusTooManyRequests,
	"RequestHeaderFieldsTooLarge":  http.StatusRequestHeaderFieldsTooLarge,
	"UnavailableForLegalReasons":   http.StatusUnavailableForLegalReasons,

	"InternalServerError":           http.StatusInternalServerError,
	"NotImplemented":                http.StatusNotImplemented,
	"BadGateway":                    http.StatusBadGateway,
	"ServiceUnavailable":            http.StatusServiceUnavailable,
	"GatewayTimeout":                http.StatusGatewayTimeout,
	"HTTPVersionNotSupported":       http.StatusHTTPVersionNotSupported,
	"VariantAlsoNegotiates":         http.StatusVariantAlsoNegotiates,
	"InsufficientStorage":           http.StatusInsufficientStorage,
	"LoopDetected":                  http.StatusLoopDetected,
	"NotExtended":                   http.StatusNotExtended,
	"NetworkAuthenticationRequired": http.StatusNetworkAuthenticationRequired,
}

// funcStatus looks up an HTTP status code by string.
func funcStatus(status string) (int, error) {
	if code, ok := validStatus[status]; ok {
		return code, nil
	}
	return 0, fmt.Errorf("invalid http status name: '%s'", status)
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
// script expression context.
func funcTrustJSStr(s string) template.JSStr {
	return template.JSStr(s)
}

// funcTrustSrcSet marks the string s as safe and does not escape its contents in
// script tag context.
func funcTrustSrcSet(s string) template.Srcset {
	return template.Srcset(s)
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

func funcTry(fn any, args ...any) (*result, error) {
	if fn == nil {
		return nil, fmt.Errorf("nil func")
	}
	fnv := reflect.ValueOf(fn)
	if fnv.Kind() != reflect.Func {
		return nil, fmt.Errorf("not a function")
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
	var out []reflect.Value
	if fnv.Type().IsVariadic() {
		out = fnv.CallSlice(reflectArgs)
	} else {
		out = fnv.Call(reflectArgs)
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
