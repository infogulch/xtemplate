package xtemplate

// This file contains different types of 'http.Handler's used by xtemplate.

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
)

var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

func bufferingTemplateHandler(server *Instance, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := GetLogger(r.Context())

		dot, err := server.bufferDot.value(server.config.Ctx, w, r)
		if err != nil {
			log.Error("failed to initialize dot value", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		buf := bufPool.Get().(*bytes.Buffer)
		buf.Reset()
		defer bufPool.Put(buf)

		err = tmpl.Execute(buf, *dot)

		if err = server.bufferDot.cleanup(dot, err); err != nil {
			log.Warn("error executing template", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Write(buf.Bytes())
	}
}

func flushingTemplateHandler(server *Instance, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := GetLogger(r.Context())

		if r.Header.Get("Accept") != "text/event-stream" {
			http.Error(w, "SSE endpoint", http.StatusNotAcceptable)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		dot, err := server.flusherDot.value(server.config.Ctx, w, r)
		if err != nil {
			log.Error("failed to initialize dot value", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		err = tmpl.Execute(w, *dot)

		if err = server.flusherDot.cleanup(dot, err); err != nil {
			log.Info("error executing template", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}
}

func staticFileHandler(fs fs.FS, fileinfo *fileInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := GetLogger(r.Context())

		urlpath := path.Clean(r.URL.Path)
		if urlpath != fileinfo.identityPath {
			// should not happen; we only add handlers for existent files
			log.LogAttrs(r.Context(), slog.LevelWarn, "tried to serve a file that doesn't exist")
			http.NotFound(w, r)
			return
		}

		// If the request provides a hash, check that it matches. If not, we don't have that file.
		queryhash := r.URL.Query().Get("hash")
		if queryhash != "" && queryhash != fileinfo.hash {
			log.LogAttrs(r.Context(), slog.LevelDebug, "request for file with wrong hash query parameter", slog.String("expected", fileinfo.hash), slog.String("queryhash", queryhash))
			http.NotFound(w, r)
			return
		}

		// negotiate encoding between the client's q value preference and fileinfo.encodings ordering (prefer earlier listed encodings first)
		encoding, err := negiotiateEncoding(r.Header["Accept-Encoding"], fileinfo.encodings)
		if err != nil {
			log.LogAttrs(r.Context(), slog.LevelWarn, "error selecting encoding to serve", slog.Any("error", err))
		}
		// we may have gotten an encoding even if there was an error; test separately
		if encoding == nil {
			http.Error(w, "internal server error", 500)
			return
		}

		log.LogAttrs(r.Context(), slog.LevelDebug, "serving file request", slog.String("encoding", encoding.encoding), slog.String("contenttype", fileinfo.contentType))
		file, err := fs.Open(encoding.path)
		if err != nil {
			log.LogAttrs(r.Context(), slog.LevelWarn, "failed to open file", slog.Any("error", err), slog.String("encoding.path", encoding.path), slog.String("requestpath", r.URL.Path))
			http.Error(w, "internal server error", 500)
			return
		}
		defer file.Close()

		// check if file was modified since loading it
		{
			stat, err := file.Stat()
			if err != nil {
				log.LogAttrs(r.Context(), slog.LevelError, "error getting stat of file", slog.Any("error", err))
			} else if modtime := stat.ModTime(); !modtime.Equal(encoding.modtime) {
				log.LogAttrs(r.Context(), slog.LevelWarn, "file maybe modified since loading", slog.Time("expected-modtime", encoding.modtime), slog.Time("actual-modtime", modtime))
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
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeContent(w, r, encoding.path, encoding.modtime, file.(io.ReadSeeker))
	}
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
