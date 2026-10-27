package xtemplate

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
)

type dotReqProvider struct{}

func (dotReqProvider) FieldName() string { return "Req" }
func (dotReqProvider) Prototype() any    { return DotReq{} }
func (dotReqProvider) Value(_ http.ResponseWriter, r *http.Request) (any, error) {
	return DotReq{r}, nil
}

var _ Provider = dotReqProvider{}

// DefaultMaxBodyBytes is the maximum request body accepted by [DotReq.ReadBody]
// when the caller does not pass a limit. It is 1 MiB.
const DefaultMaxBodyBytes int64 = 1 << 20

// DotReq is used as the .Req field for template invocations with an associated
// request, and contains the current HTTP request struct which can be used to
// read request data. See [http.Request] for detailed documentation. Some
// notable methods and fields:
//
// [http.Request.Method], [http.Request.PathValue], [http.Request.URL],
// [http.Request.URL.Query], [http.Request.Cookie], [http.Request.Header],
// [http.Request.Header.Get], [http.Request.FormValue], [DotReq.ReadBody].
//
// Note that [http.Request.ParseForm] must be called before using
// [http.Request.Form], [http.Request.PostForm], and [http.Request.PostValue].
// [DotReq.ReadBody] reads the raw body (JSON, webhooks, and other non-form
// payloads) without parsing it as a form.
type DotReq struct {
	*http.Request
}

// ReadBody reads the raw request body and returns it as a string.
//
// An optional maxBytes argument is the maximum number of bytes to accept.
// When it is omitted, the limit is [DefaultMaxBodyBytes] (1 MiB). More than
// one argument, or a limit less than or equal to zero, returns an error.
// Pass an integer literal from a template, for example {{.Req.ReadBody 4096}}.
//
// The body is read up to one byte past the limit. If it is larger than the
// limit, ReadBody returns an error wrapping [ErrorStatus] with
// [http.StatusRequestEntityTooLarge] so the buffered handler responds with
// HTTP 413 Request Entity Too Large instead of 500. A nil [DotReq], a nil
// [http.Request], or a nil body returns "" and a nil error.
//
// On success the request body is replaced with a replay of the bytes just
// read (`io.NopCloser(bytes.NewReader(buf))`). A later ReadBody in the same
// request — including a second call in the same template — returns those bytes
// again when its limit is at least that length. Because the body is restored,
// FormValue and ParseForm still see the payload when ReadBody runs first.
// Pipe the string to Sprig's fromJson for JSON payloads:
// {{.Req.ReadBody | fromJson}}.
func (r *DotReq) ReadBody(maxBytes ...int64) (string, error) {
	if r == nil || r.Request == nil || r.Body == nil {
		return "", nil
	}
	limit, err := readBodyLimit(maxBytes)
	if err != nil {
		return "", err
	}

	orig := r.Body
	// Read one extra byte so a body of exactly limit bytes succeeds and a body
	// of limit+1 is rejected without buffering the rest.
	buf, err := io.ReadAll(io.LimitReader(orig, limit+1))
	if err != nil {
		// Keep the prefix that was already pulled off orig.
		r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(buf), orig))
		return "", fmt.Errorf("ReadBody: reading request body: %w", err)
	}
	if int64(len(buf)) > limit {
		// orig still holds the unread tail. Stitch it back so a later call with
		// a large enough limit can read the full payload.
		r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(buf), orig))
		return "", fmt.Errorf("ReadBody: request body exceeds limit of %d bytes: %w", limit, ErrorStatus(http.StatusRequestEntityTooLarge))
	}
	r.Body = io.NopCloser(bytes.NewReader(buf))
	return string(buf), nil
}

func readBodyLimit(maxBytes []int64) (int64, error) {
	switch len(maxBytes) {
	case 0:
		return DefaultMaxBodyBytes, nil
	case 1:
		if maxBytes[0] <= 0 {
			return 0, fmt.Errorf("ReadBody: maxBytes must be positive, got %d", maxBytes[0])
		}
		if maxBytes[0] == math.MaxInt64 {
			// limit+1 would overflow int64 and LimitReader would treat it as EOF.
			return 0, fmt.Errorf("ReadBody: maxBytes %d is too large", maxBytes[0])
		}
		return maxBytes[0], nil
	default:
		return 0, fmt.Errorf("ReadBody: expected at most one maxBytes argument, got %d", len(maxBytes))
	}
}
