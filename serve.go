package xtemplate

import (
	"bytes"
	"context"
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

	"github.com/segmentio/ksuid"
	"golang.org/x/exp/maps"
)

func (server *xtemplate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	server.router.ServeHTTP(w, r)
}

type Handler func(w http.ResponseWriter, r *http.Request, log *slog.Logger, server *xtemplate)

func (server *xtemplate) handle(handlerPath string, handler Handler) {
	server.router.HandleFunc(handlerPath, func(w http.ResponseWriter, r *http.Request) {
		select {
		case _, _ = <-server.ctx.Done():
			server.log.Error("received request after xtemplate instance cancelled", slog.String("method", r.Method), slog.String("path", r.URL.Path))
			http.Error(w, "server stopped", http.StatusInternalServerError)
			return
		default:
		}

		start := time.Now()

		log := server.log.With(slog.Group("serve",
			slog.String("requestid", getRequestId(r.Context())),
			slog.String("method", r.Method),
			slog.String("requestPath", r.URL.Path),
			slog.String("handlerPath", handlerPath),
		))
		log.DebugContext(r.Context(), "serving request",
			slog.String("user-agent", r.Header.Get("User-Agent")),
		)

		handler(w, r, log, server)

		log.Debug("request served", slog.Duration("response-time", time.Since(start)))
	})
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

func serveTemplateHandler(tmpl *template.Template) Handler {
	return func(w http.ResponseWriter, r *http.Request, log *slog.Logger, server *xtemplate) {
		buf := bufPool.Get().(*bytes.Buffer)
		buf.Reset()
		defer bufPool.Put(buf)

		context := &struct {
			baseContext
			fsContext
			requestContext
			responseContext
		}{
			baseContext{
				log:    log,
				server: server,
			},
			fsContext{
				fs:  server.contextFS,
				log: log,
			},
			requestContext{
				Req: r,
			},
			responseContext{
				status: 200,
				Header: http.Header{},
			},
		}

		err := tmpl.Execute(buf, context)

		var handlerErr HandlerError
		if errors.As(err, &handlerErr) {
			if dberr := context.resolvePendingTx(nil); dberr != nil {
				log.Info("failed to commit transaction", slog.Any("error", dberr))
			}
			log.Debug("forwarding response handling", slog.Any("handler", handlerErr))
			handlerErr.ServeHTTP(w, r)
			return
		}
		if errors.As(err, &ReturnError{}) {
			err = nil
		}
		if err = context.resolvePendingTx(err); err != nil {
			log.Info("error executing template", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		maps.Copy(w.Header(), context.Header)
		w.WriteHeader(context.status)
		w.Write(buf.Bytes())
	}
}

func sseTemplateHandler(tmpl *template.Template) Handler {
	return func(w http.ResponseWriter, r *http.Request, log *slog.Logger, server *xtemplate) {
		if r.Header.Get("Accept") != "text/event-stream" {
			http.Error(w, "SSE endpoint", http.StatusNotAcceptable)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			log.Debug("response writer could not cast to http.Flusher")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		context := &struct {
			flushContext
			fsContext
			requestContext
		}{
			flushContext{
				flusher,
				baseContext{
					log:        log,
					server:     server,
					requestCtx: r.Context(),
				},
			},
			fsContext{
				fs:  server.contextFS,
				log: log,
			},
			requestContext{
				Req: r,
			},
		}

		err := tmpl.Execute(w, context)

		var handlerErr HandlerError
		if errors.As(err, &handlerErr) {
			if dberr := context.resolvePendingTx(nil); dberr != nil {
				log.Info("failed to commit transaction", slog.Any("error", dberr))
			}
			log.Debug("forwarding response handling", slog.Any("handler", handlerErr))
			handlerErr.ServeHTTP(w, r)
			return
		}
		if errors.As(err, &ReturnError{}) {
			err = nil
		}
		if err = context.resolvePendingTx(err); err != nil {
			log.Info("error executing template", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}
}

func serveFileHandler(w http.ResponseWriter, r *http.Request, log *slog.Logger, server *xtemplate) {
	urlpath := path.Clean(r.URL.Path)
	fileinfo, ok := server.files[urlpath]
	if !ok {
		// should not happen; we only add handlers for existent files
		log.Warn("tried to serve a file that doesn't exist", slog.String("path", urlpath), slog.String("urlpath", r.URL.Path))
		http.NotFound(w, r)
		return
	}

	// if the request provides a hash, check that it matches. if not, we don't have that file
	queryhash := r.URL.Query().Get("hash")
	if queryhash != "" && queryhash != fileinfo.hash {
		log.Debug("request for file with wrong hash query parameter", slog.String("expected", fileinfo.hash), slog.String("queryhash", queryhash))
		http.NotFound(w, r)
		return
	}

	// negotiate encoding between the client's q value preference and fileinfo.encodings ordering (prefer earlier listed encodings first)
	encoding, err := negiotiateEncoding(r.Header["Accept-Encoding"], fileinfo.encodings)
	if err != nil {
		log.Warn("error selecting encoding to serve", slog.Any("error", err))
	}
	// we may have gotten an encoding even if there was an error; test separately
	if encoding == nil {
		http.Error(w, "internal server error", 500)
		return
	}

	log.Debug("serving file request", slog.String("path", urlpath), slog.String("encoding", encoding.encoding), slog.String("contenttype", fileinfo.contentType))
	file, err := server.templateFS.Open(encoding.path)
	if err != nil {
		log.Error("failed to open file", slog.Any("error", err), slog.String("encoding.path", encoding.path), slog.String("requestpath", r.URL.Path))
		http.Error(w, "internal server error", 500)
		return
	}
	defer file.Close()

	// check if file was modified since loading it
	{
		stat, err := file.Stat()
		if err != nil {
			log.Error("error getting stat of file", slog.Any("error", err))
		} else if modtime := stat.ModTime(); !modtime.Equal(encoding.modtime) {
			log.Warn("file maybe modified since loading", slog.Time("expected-modtime", encoding.modtime), slog.Time("actual-modtime", modtime))
		}
	}

	w.Header().Add("Etag", `"`+fileinfo.hash+`"`)
	w.Header().Add("Content-Type", fileinfo.contentType)
	w.Header().Add("Content-Encoding", encoding.encoding)
	w.Header().Add("Vary", "Accept-Encoding")
	// w.Header().Add("Access-Control-Allow-Origin", "*") // ???
	if queryhash != "" {
		// cache aggressively if the request is disambiguated by a valid hash
		// should be `public` ???
		w.Header().Set("Cache-Control", "public, max-age=31536000")
	}
	http.ServeContent(w, r, encoding.path, encoding.modtime, file.(io.ReadSeeker))
}

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
