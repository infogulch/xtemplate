package xtemplate

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"path"
	"strings"
	"time"
)

// errResponseServed is returned from [dotRespProvider.Finalize] when the
// response was fully decided outside the main template buffer — either by
// [DotResp.ServeContent] (eager write) or [DotResp.RespondWith] (deferred
// write in Finalize). Buffered handlers treat it as success and must not
// write the template buffer or call http.Error.
var errResponseServed = errors.New("response already served")

type dotRespProvider struct{}

func (dotRespProvider) FieldName() string { return "Resp" }
func (dotRespProvider) Prototype() any {
	return DotResp{Header: make(http.Header)}
}
func (dotRespProvider) Value(w http.ResponseWriter, r *http.Request) (any, error) {
	return DotResp{
		Header: make(http.Header),
		status: http.StatusOK,
		w:      w, r: r,
		log: GetLogger(r.Context()),
	}, nil
}

func (dotRespProvider) Finalize(v any, err error) error {
	d := v.(DotResp)
	if d.served {
		// ServeContent already wrote the full response (including WriteHeader).
		// Do not write headers or status again.
		return errResponseServed
	}
	if d.replace {
		if err != nil {
			return err
		}
		maps.Copy(d.w.Header(), d.Header)
		// Default Content-Type only for non-empty HTML bodies when unset.
		// Empty bodies (redirects, 204) must not force a Content-Type.
		if d.replaceHTML && len(d.replaceBody) > 0 && d.w.Header().Get("Content-Type") == "" {
			d.w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		d.w.WriteHeader(d.status)
		if len(d.replaceBody) > 0 {
			_, _ = d.w.Write(d.replaceBody)
		}
		return errResponseServed
	}
	var errSt ErrorStatus
	if errors.As(err, &errSt) {
		// Apply intended client status; handler must not overwrite with 500.
		maps.Copy(d.w.Header(), d.Header)
		d.w.WriteHeader(int(errSt))
	} else if err == nil {
		maps.Copy(d.w.Header(), d.Header)
		d.w.WriteHeader(d.status)
	}
	return err
}

var _ Finalizer = dotRespProvider{}

// DotResp is used as the .Resp field in buffered template invocations.
type DotResp struct {
	http.Header
	status int
	// served is true when ServeContent already wrote the full response to w.
	served bool
	// replace is true when RespondWith should commit status+body in Finalize
	// and skip the main template buffer.
	replace     bool
	replaceBody []byte
	replaceHTML bool
	w           http.ResponseWriter
	r           *http.Request
	log         *slog.Logger
}

// ServeContent aborts execution of the template and instead responds to the
// request with content with any headers set by AddHeader and SetHeader so far
// but ignoring SetStatus. content must be a string, []byte, or io.ReadSeeker.
func (d *DotResp) ServeContent(path_ string, modtime time.Time, content any) (string, error) {
	var reader io.ReadSeeker
	switch c := content.(type) {
	case nil:
		return "", fmt.Errorf("ServeContent: content is nil")
	case string:
		reader = strings.NewReader(c)
	case []byte:
		reader = bytes.NewReader(c)
	case io.ReadSeeker:
		reader = c
	default:
		return "", fmt.Errorf("ServeContent: unsupported content type %T (want string, []byte, or io.ReadSeeker)", content)
	}
	path_ = path.Clean(path_)
	d.log.Debug("serving content response", slog.String("path", path_))
	maps.Copy(d.w.Header(), d.Header)
	http.ServeContent(d.w, d.r, path_, modtime, reader)
	d.served = true
	return "", ReturnError{}
}

// RespondWith discards any template output buffered so far and finishes the
// request with the given HTTP status and body. Unlike [DotResp.ReturnStatus],
// the template buffer is not sent. body is required (pass "" for an empty
// body). Supported types: string, template.HTML, []byte.
//
// Headers set on .Resp before RespondWith (Location, Content-Type, etc.) are
// kept. If body is template.HTML, non-empty, and Content-Type is unset, it
// defaults to text/html; charset=utf-8. Empty bodies do not force a
// Content-Type.
func (d *DotResp) RespondWith(status int, body any) (string, error) {
	var b []byte
	var isHTML bool
	switch c := body.(type) {
	case nil:
		return "", fmt.Errorf("RespondWith: body is nil")
	case string:
		b = []byte(c)
	case template.HTML:
		b = []byte(c)
		isHTML = true
	case []byte:
		// Copy so later mutation of the caller's slice cannot affect the response.
		b = bytes.Clone(c)
	default:
		return "", fmt.Errorf("RespondWith: unsupported body type %T (want string, template.HTML, or []byte)", body)
	}
	d.status = status
	d.replace = true
	d.replaceBody = b
	d.replaceHTML = isHTML
	d.log.Debug("respond-with replacement response", slog.Int("status", status), slog.Int("body_len", len(b)))
	return "", ReturnError{}
}

// AddHeader adds a header field value, appending val to
// existing values for that field. It returns an
// empty string.
func (h *DotResp) AddHeader(field, val string) string {
	h.Add(field, val)
	return ""
}

// SetHeader sets a header field value, overwriting any
// other values for that field. It returns an
// empty string.
func (h *DotResp) SetHeader(field, val string) string {
	h.Set(field, val)
	return ""
}

// DelHeader deletes a header field. It returns an empty string.
func (h *DotResp) DelHeader(field string) string {
	h.Del(field)
	return ""
}

// SetStatus sets the HTTP response status. It returns an empty string.
func (h *DotResp) SetStatus(status int) string {
	h.status = status
	return ""
}

// ReturnStatus sets the HTTP response status and exits template rendering
// immediately. Unlike [DotResp.RespondWith], the template buffer is kept and
// written as the response body.
func (h *DotResp) ReturnStatus(status int) (string, error) {
	h.status = status
	return "", ReturnError{}
}

type ErrorStatus int

func (e ErrorStatus) Error() string {
	return http.StatusText(int(e))
}
