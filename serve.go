package xtemplate

import (
	"bytes"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

func (t *XTemplate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	runtime := t.runtime // copy the runtime in case it's updated during the request

	_, template, params, _ := runtime.router.Find(r.Method, r.URL.Path)
	if template == nil {
		t.Log.Debug("no handler for request", "method", r.Method, "path", r.URL.Path)
		http.NotFound(w, r)
		return
	}

	log := t.Log.With(slog.Group("serve",
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("user-agent", r.Header.Get("User-Agent")),
	))
	log.Debug("found response template",
		slog.String("template-name", template.Name()),
		slog.Any("params", params),
		slog.DurationValue(time.Since(start)))

	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	var tx *sql.Tx
	var err error
	if t.DB != nil {
		tx, err = t.DB.Begin()
		if err != nil {
			log.Info("failed to begin database transaction", "error", err)
			http.Error(w, "unable to connect to the database", http.StatusInternalServerError)
			return
		}
	}

	var headers = http.Header{}
	context := &TemplateContext{
		Req:     r,
		Params:  params,
		Headers: WrappedHeader{headers},
		Config:  t.Config,

		status: http.StatusOK,
		tmpl:   runtime.tmpl,
		funcs:  runtime.funcs,
		fs:     t.ContextFS,
		log:    log,
		tx:     tx,
	}

	r.ParseForm()
	err = template.Execute(buf, context)

	headers.Set("Content-Type", "text/html; charset=utf-8")
	headers.Set("Content-Length", strconv.Itoa(buf.Len()))
	headers.Del("Accept-Ranges") // we don't know ranges for dynamically-created content
	headers.Del("Last-Modified") // useless for dynamic content since it's always changing

	// we don't know a way to quickly generate etag for dynamic content,
	// and weak etags still cause browsers to rely on it even after a
	// refresh, so disable them until we find a better way to do this
	headers.Del("Etag")

	var returnErr ReturnError
	if err != nil && !errors.As(err, &returnErr) {
		var handlerErr HandlerError
		if errors.As(err, &handlerErr) {
			if dberr := tx.Commit(); dberr != nil {
				log.Info("failed to commit transaction", "error", dberr)
			}
			log.Debug("forwarding response handling", "handler", handlerErr)
			handlerErr.ServeHTTP(w, r)
			return
		}
		log.Info("error executing template", "error", err)
		if tx != nil {
			if dberr := tx.Rollback(); dberr != nil {
				log.Info("failed to roll back transaction", "error", dberr)
			}
		}
		http.Error(w, "failed to render response", http.StatusInternalServerError)
		return
	} else if tx != nil {
		if dberr := tx.Commit(); dberr != nil {
			log.Info("failed to commit transaction", "error", dberr)
			http.Error(w, "failed to commit database transaction", http.StatusInternalServerError)
			return
		}
	}

	wheader := w.Header()
	for name, values := range headers {
		for _, value := range values {
			wheader.Add(name, value)
		}
	}
	w.WriteHeader(context.status)
	w.Write(buf.Bytes())

	log.Debug("done", slog.DurationValue(time.Since(start)), slog.Int("status", context.status))
}

// ReturnError is a sentinel value returned by the `return` template
// func/keyword that indicates a successful/normal exit but allows the template
// to exit early.
type ReturnError struct{}

func (ReturnError) Error() string { return "returned" }

var _ = (error)((*ReturnError)(nil))

// HandlerError is a special error that hijacks the normal response handling and
// passes response handling off to the ServeHTTP method on this error value.
type HandlerError interface {
	Error() string
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// interface guard
var _ = (error)((HandlerError)(nil))

type funcHandlerError struct {
	name string
	fn   func(w http.ResponseWriter, r *http.Request)
}

// NewHandlerError returns a new HandlerError based on a string and a function
// that matches the ServeHTTP signature.
func NewHandlerError(name string, fn func(w http.ResponseWriter, r *http.Request)) HandlerError {
	return funcHandlerError{name, fn}
}

func (fhe funcHandlerError) Error() string { return fhe.name }

func (fhe funcHandlerError) ServeHTTP(w http.ResponseWriter, r *http.Request) { fhe.fn(w, r) }
