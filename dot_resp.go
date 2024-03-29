package xtemplate

import (
	"io"
	"log/slog"
	"maps"
	"net/http"
	"path"
	"strings"
	"time"
)

type dotRespProvider struct{}

func (dotRespProvider) Value(r Request) (any, error) {
	return DotResp{
		Header: make(http.Header),
		status: http.StatusOK,
		w:      r.W, r: r.R,
		log: GetCtxLogger(r.R),
	}, nil
}

func (dotRespProvider) Cleanup(v any, err error) error {
	d := v.(DotResp)
	if err == nil {
		maps.Copy(d.w.Header(), d.Header)
		d.w.WriteHeader(d.status)
	}
	return err
}

var _ CleanupDotProvider = dotRespProvider{}

// DotResp is used as the .Resp field in buffered template invocations.
type DotResp struct {
	http.Header
	status int
	w      http.ResponseWriter
	r      *http.Request
	log    *slog.Logger
}

// ServeContent aborts execution of the template and instead responds to the
// request with content with any headers set by AddHeader and SetHeader so far
// but ignoring SetStatus.
func (d *DotResp) ServeContent(path_ string, modtime time.Time, content any) (string, error) {
	var reader io.ReadSeeker
	switch c := content.(type) {
	case string:
		reader = strings.NewReader(c)
	case io.ReadSeeker:
		reader = c
	}
	path_ = path.Clean(path_)
	d.log.Debug("serving content response", slog.String("path", path_))
	maps.Copy(d.w.Header(), d.Header)
	http.ServeContent(d.w, d.r, path_, modtime, reader)
	return "", ReturnError{}
}

// AddHeader adds a header field value, appending val to
// existing values for that field. It returns an
// empty string.
func (h *DotResp) AddHeader(field, val string) string {
	h.Header.Add(field, val)
	return ""
}

// SetHeader sets a header field value, overwriting any
// other values for that field. It returns an
// empty string.
func (h *DotResp) SetHeader(field, val string) string {
	h.Header.Set(field, val)
	return ""
}

// DelHeader deletes a header field. It returns an empty string.
func (h *DotResp) DelHeader(field string) string {
	h.Header.Del(field)
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
