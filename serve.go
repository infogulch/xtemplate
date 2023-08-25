package xtemplate

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

func (t *Templates) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	logger := t.ctx.Logger().Named(fmt.Sprintf("request-%s", caddyhttp.GetVar(r.Context(), "uuid").(fmt.Stringer)))
	handle, params, _ := t.router.Lookup(r.Method, r.URL.Path)
	if handle == nil {
		logger.Debug("no handler for request", zap.String("method", r.Method), zap.String("path", r.URL.Path))
		return caddyhttp.Error(http.StatusNotFound, nil)
	}
	var template *template.Template
	handle(nil, new(http.Request).WithContext(context.WithValue(context.Background(), "🙈", &template)), nil)
	logger.Debug("handling request", zap.String("method", r.Method), zap.String("path", r.URL.Path), zap.Any("params", params), zap.String("name", template.Name()))

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
			logger.Info("failed to begin database transaction", zap.Error(err))
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
		Next:       next,
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
				logger.Info("error committing transaction", zap.Error(err))
			}
			return handlerErr
		}
		logger.Info("error executing template", zap.Error(err))
		if dberr := tx.Rollback(); dberr != nil {
			logger.Info("error rolling back transaction", zap.Error(err))
		}
		return caddyhttp.Error(http.StatusInternalServerError, err)
	} else {
		if dberr := tx.Commit(); dberr != nil {
			logger.Info("error committing transaction", zap.Error(err))
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
