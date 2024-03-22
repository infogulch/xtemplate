package xtemplate

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"path"
	"reflect"
	"strings"
	"time"
)

type responseDotProvider struct{}

func (responseDotProvider) Type() reflect.Type { return reflect.TypeOf(ResponseDot{}) }

func (responseDotProvider) Value(log *slog.Logger, _ context.Context, w http.ResponseWriter, r *http.Request) (reflect.Value, error) {
	return reflect.ValueOf(ResponseDot{Header: make(http.Header), status: http.StatusOK, w: w, r: r, log: log}), nil
}

func (responseDotProvider) Cleanup(v reflect.Value, err error) error {
	d := v.Interface().(ResponseDot)
	var handlerErr HandlerError
	if err == nil {
		maps.Copy(d.w.Header(), d.Header)
		d.w.WriteHeader(d.status)
		return nil
	} else if errors.As(err, &handlerErr) {
		d.log.Debug("forwarding response handling", slog.Any("handler", handlerErr))
		handlerErr.ServeHTTP(d.w, d.r)
		return nil
	} else {
		return err
	}
}

var _ CleanupDotProvider = responseDotProvider{}

type ResponseDot struct {
	http.Header
	status int
	w      http.ResponseWriter
	r      *http.Request
	log    *slog.Logger
}

// ServeContent aborts execution of the template and instead responds to the request with content
func (d *ResponseDot) ServeContent(path_ string, modtime time.Time, content string) (string, error) {
	return "", NewHandlerError("ServeContent", func(w http.ResponseWriter, r *http.Request) {
		path_ = path.Clean(path_)

		d.log.Debug("serving content response", slog.String("path", path_))

		http.ServeContent(w, r, path_, modtime, strings.NewReader(content))
	})
}

// AddHeader adds a header field value, appending val to
// existing values for that field. It returns an
// empty string.
func (h ResponseDot) AddHeader(field, val string) string {
	h.Header.Add(field, val)
	return ""
}

// SetHeader sets a header field value, overwriting any
// other values for that field. It returns an
// empty string.
func (h ResponseDot) SetHeader(field, val string) string {
	h.Header.Set(field, val)
	return ""
}

// DelHeader deletes a header field. It returns an empty string.
func (h ResponseDot) DelHeader(field string) string {
	h.Header.Del(field)
	return ""
}

// SetStatus sets the HTTP response status. It returns an empty string.
func (h *ResponseDot) SetStatus(status int) string {
	h.status = status
	return ""
}

// ReturnStatus sets the HTTP response status and exits template rendering
// immediately.
func (h *ResponseDot) ReturnStatus(status int) (string, error) {
	h.status = status
	return "", ReturnError{}
}
