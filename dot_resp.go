package xtemplate

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"path"
	"strings"
	"time"
)

// errResponseServed is returned from [dotRespProvider.Finalize] when
// [DotResp.ServeContent] already wrote the full response. Buffered handlers
// treat it as success and must not write the template buffer or call http.Error.
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
		// The response was already fully written by ServeContent (which calls
		// WriteHeader itself); writing headers or status again here would cause
		// a superfluous WriteHeader call. Signal the handler to skip the buffer.
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
	served bool
	w      http.ResponseWriter
	r      *http.Request
	log    *slog.Logger
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
// immediately.
func (h *DotResp) ReturnStatus(status int) (string, error) {
	h.status = status
	return "", ReturnError{}
}

type ErrorStatus int

func (e ErrorStatus) Error() string {
	return http.StatusText(int(e))
}
