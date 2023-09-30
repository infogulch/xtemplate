package xtemplate

import (
	"bytes"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

func (t *Templates) ServeHTTP(w http.ResponseWriter, r *http.Request, _ caddyhttp.Handler) error {
	logger := t.log.WithGroup("xtemplate-render").With("method", r.Method, "path", r.URL.Path)
	template, params, _, _ := t.router.LookupEndpoint(r.Method, r.URL.Path)
	if template == nil {
		logger.Debug("no handler for request")
		return caddyhttp.Error(http.StatusNotFound, nil)
	}
	logger = logger.With("params", params, "name", template.Name())
	logger.Debug("handling request")

	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	alwaysbuffer := func(_ int, _ http.Header) bool { return true }
	rec := caddyhttp.NewResponseRecorder(w, buf, alwaysbuffer)

	var tx *sql.Tx
	var err error
	if t.DB != nil {
		tx, err = t.DB.Begin()
		if err != nil {
			logger.Info("failed to begin database transaction", "error", err)
			return caddyhttp.Error(http.StatusInternalServerError, err)
		}
	}

	r.ParseForm()
	var statusCode = 200
	context := &TemplateContext{
		Req:        r,
		Params:     params,
		RespStatus: func(c int) string { statusCode = c; return "" },
		RespHeader: WrappedHeader{w.Header()},
		Config:     t.Config,

		tmpl:  t.tmpl,
		funcs: t.funcs,
		fs:    t.ContextFS,
		log:   logger,
		tx:    tx,
	}

	err = template.Execute(w, context)
	if err != nil {
		var handlerErr caddyhttp.HandlerError
		if errors.As(err, &handlerErr) {
			if dberr := tx.Commit(); dberr != nil {
				logger.Info("failed to commit transaction", "error", err)
			}
			return handlerErr
		}
		logger.Info("error executing template", zap.Error(err))
		if dberr := tx.Rollback(); dberr != nil {
			logger.Info("failed to roll back transaction", "error", err)
		}
		return caddyhttp.Error(http.StatusInternalServerError, err)
	} else {
		if dberr := tx.Commit(); dberr != nil {
			logger.Info("error committing transaction", "error", err)
		}
	}

	rec.WriteHeader(statusCode)
	rec.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	rec.Header().Del("Accept-Ranges") // we don't know ranges for dynamically-created content
	rec.Header().Del("Last-Modified") // useless for dynamic content since it's always changing

	// we don't know a way to quickly generate etag for dynamic content,
	// and weak etags still cause browsers to rely on it even after a
	// refresh, so disable them until we find a better way to do this
	rec.Header().Del("Etag")

	return rec.WriteResponse()
}
