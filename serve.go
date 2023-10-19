package xtemplate

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"math"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/infogulch/pathmatcher"
	"github.com/segmentio/ksuid"
)

func (x *xtemplate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	_, handler, params, _ := x.router.Find(r.Method, r.URL.Path)
	if handler == nil {
		x.log.Debug("no handler for request", "method", r.Method, "path", r.URL.Path)
		http.NotFound(w, r)
		return
	}

	log := x.log.With(slog.Group("serving",
		slog.String("requestid", getRequestId(r.Context())),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
	))
	log.DebugContext(r.Context(), "serving request",
		slog.Any("params", params),
		slog.Duration("handler-lookup-duration", time.Since(start)),
		slog.String("user-agent", r.Header.Get("User-Agent")),
	)

	ctx := context.WithValue(r.Context(), ctxKey{}, ctxValue{params: params, log: log, runtime: x})
	handler.ServeHTTP(w, r.WithContext(ctx))

	log.Debug("request served", slog.Duration("response-duration", time.Since(start)))
}

func getRequestId(ctx context.Context) string {
	// caddy request id
	if v := ctx.Value("vars"); v != nil {
		if mv, ok := v.(map[string]any); ok {
			if anyrid, ok := mv["uuid"]; ok {
				if rid, ok := anyrid.(string); ok {
					return rid
				}
			}
		}
	}
	return ksuid.New().String()
}

type ctxKey struct{}

type ctxValue struct {
	params  pathmatcher.Params
	log     *slog.Logger
	runtime *xtemplate
}

func getContext(ctx context.Context) (pathmatcher.Params, *slog.Logger, *xtemplate) {
	val := ctx.Value(ctxKey{}).(ctxValue)
	return val.params, val.log, val.runtime
}

func serveTemplateHandler(tmpl *template.Template) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params, log, runtime := getContext(r.Context())

		buf := bufPool.Get().(*bytes.Buffer)
		buf.Reset()
		defer bufPool.Put(buf)

		var tx *sql.Tx
		var err error
		if runtime.db != nil {
			tx, err = runtime.db.Begin()
			if err != nil {
				log.Info("failed to begin database transaction", "error", err)
				http.Error(w, "unable to connect to the database", http.StatusInternalServerError)
				return
			}
		}

		var headers = http.Header{}
		context := &TemplateContext{
			Req:    r,
			Params: params,

			Headers: WrappedHeader{headers},
			status:  http.StatusOK,

			log:     log,
			tx:      tx,
			runtime: runtime,
		}

		r.ParseForm()
		err = tmpl.Execute(buf, context)

		log.Debug("executed template", slog.Any("template error", err), slog.Int("length", buf.Len()))

		var returnErr ReturnError
		if err != nil && !errors.As(err, &returnErr) {
			var handlerErr HandlerError
			if errors.As(err, &handlerErr) {
				if tx != nil {
					if dberr := tx.Commit(); dberr != nil {
						log.Info("failed to commit transaction", "error", dberr)
					}
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

		wheader.Set("Content-Type", "text/html; charset=utf-8")
		wheader.Set("Content-Length", strconv.Itoa(buf.Len()))
		wheader.Del("Accept-Ranges") // we don't know ranges for dynamically-created content
		wheader.Del("Last-Modified") // useless for dynamic content since it's always changing

		// we don't know a way to quickly generate etag for dynamic content,
		// and weak etags still cause browsers to rely on it even after a
		// refresh, so disable them until we find a better way to do this
		wheader.Del("Etag")

		w.WriteHeader(context.status)
		w.Write(buf.Bytes())
	})
}

var serveFileHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	_, log, runtime := getContext(r.Context())

	urlpath := path.Clean(r.URL.Path)
	fileinfo, ok := runtime.files[urlpath]
	if !ok {
		// should not happen; we only add handlers for existent files
		log.Error("tried to serve a file that doesn't exist", slog.String("path", urlpath), slog.String("urlpath", r.URL.Path))
		http.NotFound(w, r)
		return
	}

	// if the request provides a hash, check that it matches. if not, we don't have that file
	if queryhash := r.URL.Query().Get("hash"); queryhash != "" && queryhash != fileinfo.hash {
		log.Debug("request for file with wrong hash query parameter", slog.String("expected", fileinfo.hash), slog.String("queryhash", queryhash))
		http.NotFound(w, r)
		return
	}

	// negotiate encoding between the client's q value preference and fileinfo.encodings ordering (prefer earlier listed encodings first)
	encoding, err := negiotiateEncoding(r.Header["Accept-Encoding"], fileinfo.encodings)
	if err != nil {
		log.Error("error selecting encoding to serve", slog.Any("error", err))
	}
	// we may have gotten an encoding even if there was an error; test separately
	if encoding == nil {
		http.Error(w, "internal server error", 500)
		return
	}

	log.Debug("serving file request", slog.String("path", urlpath), slog.String("encoding", encoding.encoding), slog.String("contenttype", fileinfo.contentType))
	file, err := runtime.templateFS.Open(encoding.path)
	if err != nil {
		log.Debug("failed to open file", slog.Any("error", err), slog.String("encoding.path", encoding.path), slog.String("requestpath", r.URL.Path))
		http.Error(w, "internal server error", 500)
		return
	}
	defer file.Close()

	// check if file was modified since loading it
	{
		stat, err := file.Stat()
		if err != nil {
			log.Debug("error getting stat of file", slog.Any("error", err))
		} else if modtime := stat.ModTime(); !modtime.Equal(encoding.modtime) {
			log.Error("file maybe modified since loading", slog.Time("expected-modtime", encoding.modtime), slog.Time("actual-modtime", modtime))
		}
	}

	w.Header().Add("Etag", `"`+fileinfo.hash+`"`)
	w.Header().Add("Content-Type", fileinfo.contentType)
	w.Header().Add("Content-Encoding", encoding.encoding)
	w.Header().Add("Vary", "Accept-Encoding")
	// w.Header().Add("Access-Control-Allow-Origin", "*") // ???
	if r.URL.Query().Get("hash") != "" {
		// cache aggressively if the request is disambiguated by a valid hash
		// should be `public` ???
		w.Header().Set("Cache-Control", "public, max-age=31536000")
	}
	http.ServeContent(w, r, encoding.path, encoding.modtime, file.(io.ReadSeeker))
})

func negiotiateEncoding(acceptHeaders []string, encodings []encodingInfo) (*encodingInfo, error) {
	var err error
	// shortcuts
	if len(encodings) == 0 {
		return nil, fmt.Errorf("impossible condition, fileInfo contains no encodings")
	}
	if len(encodings) == 1 {
		if encodings[0].encoding != "identity" {
			// identity should always be present, but return whatever we got anyway
			err = fmt.Errorf("identity encoding missing")
		}
		return &encodings[0], err
	}

	// default to identity encoding, q = 0.0
	var maxq float64
	var maxqIdx int = -1
	for i, e := range encodings {
		if e.encoding == "identity" {
			maxqIdx = i
			break
		}
	}
	if maxqIdx == -1 {
		err = fmt.Errorf("identity encoding missing")
		maxqIdx = len(encodings) - 1
	}

	for _, header := range acceptHeaders {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}
		for _, requestedEncoding := range strings.Split(header, ",") {
			requestedEncoding = strings.TrimSpace(requestedEncoding)
			if requestedEncoding == "" {
				continue
			}

			parts := strings.Split(requestedEncoding, ";")
			encpart := strings.TrimSpace(parts[0])
			requestedIdx := -1

			// find out if we can provide that encoding
			for i, e := range encodings {
				if e.encoding == encpart {
					requestedIdx = i
					break
				}
			}
			if requestedIdx == -1 {
				continue // we don't support that encoding, try next
			}

			// determine q value
			q := 1.0 // default 1.0
			for _, part := range parts[1:] {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "q=") {
					part = strings.TrimSpace(strings.TrimPrefix(part, "q="))
					if parsed, err := strconv.ParseFloat(part, 64); err == nil {
						q = parsed
						break
					}
				}
			}

			// use this encoding over previously selected encoding if:
			// 1. client has a strong preference for this encoding, OR
			// 2. client's preference is small and this encoding is listed earlier
			if q-maxq > 0.1 || (math.Abs(q-maxq) <= 0.1 && requestedIdx < maxqIdx) {
				maxq = q
				maxqIdx = requestedIdx
			}
		}
	}
	return &encodings[maxqIdx], err
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

// NewHandlerError returns a new HandlerError based on a string and a function
// that matches the ServeHTTP signature.
func NewHandlerError(name string, fn func(w http.ResponseWriter, r *http.Request)) HandlerError {
	return funcHandlerError{name, fn}
}

type funcHandlerError struct {
	name string
	fn   func(w http.ResponseWriter, r *http.Request)
}

func (fhe funcHandlerError) Error() string { return fhe.name }

func (fhe funcHandlerError) ServeHTTP(w http.ResponseWriter, r *http.Request) { fhe.fn(w, r) }
