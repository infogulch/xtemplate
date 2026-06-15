package xtemplate

// This file contains different types of 'http.Handler's used by xtemplate.

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"math"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/afero"
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

		_, _ = w.Write(buf.Bytes())
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

func staticFileHandler(fs afero.Fs, fileinfo *fileInfo) http.HandlerFunc {
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
		encoding, err := negotiateEncoding(r.Header["Accept-Encoding"], fileinfo.encodings)
		if encoding == nil {
			// The client refused every encoding we can serve (e.g. identity;q=0
			// with no acceptable alternative); per RFC 7231 respond 406.
			log.LogAttrs(r.Context(), slog.LevelDebug, "no acceptable encoding for request", slog.Any("error", err))
			http.Error(w, "not acceptable", http.StatusNotAcceptable)
			return
		}
		if err != nil {
			// We still selected an encoding despite an anomaly (e.g. identity missing).
			log.LogAttrs(r.Context(), slog.LevelWarn, "encoding negotiation anomaly", slog.Any("error", err))
		}

		log.LogAttrs(r.Context(), slog.LevelDebug, "serving file request", slog.String("encoding", encoding.encoding), slog.String("contenttype", fileinfo.contentType))
		file, err := fs.Open(encoding.path)
		if err != nil {
			log.LogAttrs(r.Context(), slog.LevelWarn, "failed to open file", slog.Any("error", err), slog.String("encoding.path", encoding.path), slog.String("requestpath", r.URL.Path))
			http.Error(w, "internal server error", 500)
			return
		}
		defer func() { _ = file.Close() }()

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
		if encoding.encoding != "identity" {
			w.Header().Add("Content-Encoding", encoding.encoding)
		}
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

// negotiateEncoding selects which of the available encodings to serve based on
// the request's Accept-Encoding header(s), following RFC 7231 §5.3.4. It honors
// q values (q=0 means "not acceptable") and the "*" wildcard. identity is
// acceptable by default unless specifically refused via identity;q=0 or, when no
// identity entry is present, *;q=0.
//
// Among acceptable encodings the highest q wins; q values within 0.1 of each
// other are treated as a tie and broken toward the encoding listed earlier in
// encodings (which is size-ascending, so smaller payloads are preferred). It
// returns nil when the client accepts nothing we can serve, so the caller can
// respond 406 Not Acceptable.
func negotiateEncoding(acceptHeaders []string, encodings []encodingInfo) (*encodingInfo, error) {
	if len(encodings) == 0 {
		return nil, fmt.Errorf("impossible condition, fileInfo contains no encodings")
	}

	// Parse the header(s) into explicit per-coding q values plus an optional
	// wildcard q that applies to any coding not named explicitly.
	explicit := map[string]float64{}
	starQ, hasStar := 0.0, false
	for _, header := range acceptHeaders {
		for _, tok := range strings.Split(header, ",") {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			parts := strings.Split(tok, ";")
			coding := strings.TrimSpace(parts[0])
			if coding == "" {
				continue
			}
			q := 1.0 // default when no q parameter is given
			for _, p := range parts[1:] {
				if v, ok := strings.CutPrefix(strings.TrimSpace(p), "q="); ok {
					if parsed, perr := strconv.ParseFloat(strings.TrimSpace(v), 64); perr == nil {
						q = parsed
						break
					}
				}
			}
			if coding == "*" {
				starQ, hasStar = q, true
			} else {
				explicit[coding] = q
			}
		}
	}

	// qOf returns the negotiated q for an available coding and whether it is
	// acceptable. identity gets a 0.0 baseline (so any explicitly-requested coding
	// outranks it) but remains acceptable unless specifically refused.
	qOf := func(coding string) (q float64, acceptable bool) {
		if v, ok := explicit[coding]; ok {
			return v, v > 0
		}
		if hasStar {
			return starQ, starQ > 0
		}
		if coding == "identity" {
			return 0, true
		}
		return 0, false
	}

	identityIdx := -1
	for i, e := range encodings {
		if e.encoding == "identity" {
			identityIdx = i
			break
		}
	}

	var err error
	bestIdx, bestQ := -1, 0.0
	if identityIdx >= 0 {
		if q, ok := qOf("identity"); ok {
			bestIdx, bestQ = identityIdx, q
		}
	} else {
		// identity should always be present for served files; note the anomaly
		// but still try to pick an acceptable alternative below.
		err = fmt.Errorf("identity encoding missing")
	}

	for i, e := range encodings {
		if i == identityIdx {
			continue
		}
		q, ok := qOf(e.encoding)
		if !ok {
			continue
		}
		if bestIdx == -1 {
			bestIdx, bestQ = i, q
			continue
		}
		if q-bestQ > 0.1 || (math.Abs(q-bestQ) <= 0.1 && i < bestIdx) {
			bestIdx, bestQ = i, q
		}
	}

	if bestIdx == -1 {
		if err == nil {
			err = fmt.Errorf("no acceptable encoding")
		}
		return nil, err
	}
	return &encodings[bestIdx], err
}
