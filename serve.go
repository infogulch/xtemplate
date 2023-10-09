package xtemplate

import (
	"bytes"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
)

func (t *XTemplate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := t.Log.WithGroup("xtemplate-render").With("method", r.Method, "path", r.URL.Path)
	runtime := t.runtime
	_, template, params, _ := runtime.router.Find(r.Method, r.URL.Path)
	if template == nil {
		log.Debug("no handler for request")
		http.NotFound(w, r)
		return
	}
	log = log.With("params", params, "name", template.Name())
	log.Debug("handling request")

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

	var statusCode = http.StatusOK
	var headers = http.Header{}
	context := &TemplateContext{
		Req:        r,
		Params:     params,
		RespStatus: func(c int) string { statusCode = c; return "" },
		RespHeader: WrappedHeader{headers},
		Config:     t.Config,

		tmpl:  runtime.tmpl,
		funcs: runtime.funcs,
		fs:    t.ContextFS,
		log:   log,
		tx:    tx,
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
		if dberr := tx.Rollback(); dberr != nil {
			log.Info("failed to roll back transaction", "error", err)
		}
		http.Error(w, "failed to render response", http.StatusInternalServerError)
		return
	} else {
		if dberr := tx.Commit(); dberr != nil {
			log.Info("failed to commit transaction", "error", dberr)
			http.Error(w, "failed to commit database transaction", http.StatusInternalServerError)
			return
		}
	}

	for name, values := range headers {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.WriteHeader(statusCode)
	w.Write(buf.Bytes())
}

type ReturnError struct{}

func (ReturnError) Error() string { return "returned" }

var _ = (error)((*ReturnError)(nil))

type HandlerError interface {
	Error() string
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// interface guard
var _ = (error)((HandlerError)(nil))

type FuncHandlerError struct {
	name string
	fn   func(w http.ResponseWriter, r *http.Request)
}

func NewHandlerError(name string, fn func(w http.ResponseWriter, r *http.Request)) HandlerError {
	return FuncHandlerError{name, fn}
}

func (fhe FuncHandlerError) Error() string { return fhe.name }

func (fhe FuncHandlerError) ServeHTTP(w http.ResponseWriter, r *http.Request) { fhe.fn(w, r) }
