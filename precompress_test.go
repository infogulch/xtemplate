package xtemplate

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

func TestPrecompress_BadEncoding(t *testing.T) {
	fs := newMemFS(t, map[string]string{"style.css": "body{}"})
	cfg := New()
	cfg.Precompress = []string{"lzma"} // unsupported
	_, _, _, err := cfg.Instance(WithTemplateFS(fs))
	if err == nil {
		t.Fatal("expected error for unsupported encoding, got nil")
	}
}

func TestPrecompress_ServesCompressedVariants(t *testing.T) {
	const css = "body { color: red; font-size: 16px; }\n"
	fs := newMemFS(t, map[string]string{"style.css": css})

	cfg := New()
	cfg.Precompress = []string{"gzip", "br", "zstd"}
	inst, _, _, err := cfg.Instance(WithTemplateFS(fs))
	if err != nil {
		t.Fatalf("failed to build instance: %v", err)
	}

	cases := []struct {
		encoding string
		reader   func(io.Reader) io.Reader
	}{
		{encoding: "", reader: func(r io.Reader) io.Reader { return bufio.NewReader(r) }},
		{encoding: "gzip", reader: func(r io.Reader) io.Reader {
			gr, err := gzip.NewReader(r)
			if err != nil {
				t.Fatalf("gzip: failed to create reader: %v", err)
			}
			return gr
		}},
		{encoding: "br", reader: func(r io.Reader) io.Reader { return brotli.NewReader(r) }},
		{encoding: "zstd", reader: func(r io.Reader) io.Reader {
			zr, err := zstd.NewReader(r)
			if err != nil {
				t.Fatalf("zstd: failed to create reader: %v", err)
			}
			return zr
		}},
	}

	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/style.css", nil)
		if tc.encoding != "" {
			r.Header.Set("Accept-Encoding", tc.encoding)
		}
		w := httptest.NewRecorder()
		inst.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("identity: status = %d, want %d", w.Code, http.StatusOK)
		}
		if ce := w.Header().Get("Content-Encoding"); ce != tc.encoding {
			t.Errorf("identity: Content-Encoding = %q, want %q", ce, tc.encoding)
		}
		reader := tc.reader(w.Body)
		if got, err := io.ReadAll(reader); err != nil {
			t.Errorf("identity: read failed: %v", err)
		} else if string(got) != css {
			t.Errorf("identity: body = %q, want %q", string(got), css)
		}
	}
}

func TestPrecompress_DoesNotServeUnconfiguredVariant(t *testing.T) {
	const css = "body { color: red; font-size: 16px; }\n"
	fs := newMemFS(t, map[string]string{"style.css": css})

	cfg := New()
	cfg.Precompress = []string{"gzip"}
	inst, _, _, err := cfg.Instance(WithTemplateFS(fs))
	if err != nil {
		t.Fatalf("failed to build instance: %v", err)
	}

	// Accept-Encoding: br → identity encoding when brotli is not configured.
	{
		wb := httptest.NewRecorder()
		rb := httptest.NewRequest(http.MethodGet, "/style.css", nil)
		rb.Header.Set("Accept-Encoding", "br")
		inst.ServeHTTP(wb, rb)
		if wb.Code != http.StatusOK {
			t.Fatalf("br-missing: status = %d, want %d", wb.Code, http.StatusOK)
		}
		if ce := wb.Header().Get("Content-Encoding"); ce != "" {
			t.Errorf("br-missing: Content-Encoding = %q, want %q", ce, "br")
		}
		got, err := io.ReadAll(wb.Body)
		if err != nil {
			t.Fatalf("br-missing: reading failed: %v", err)
		}
		if string(got) != css {
			t.Errorf("br-missing: body = %q, want %q", got, css)
		}
	}
}

func TestPrecompress_SkipsExistingCompressedFile(t *testing.T) {
	const css = "body { color: red; }\n"

	// Pre-generate a valid gzip of the CSS content.
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	if _, err := gw.Write([]byte(css)); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	fs := newMemFS(t, map[string]string{
		"style.css":    css,
		"style.css.gz": gzBuf.String(),
	})

	cfg := New()
	cfg.Precompress = []string{"gzip"}
	inst, _, _, err := cfg.Instance(WithTemplateFS(fs))
	if err != nil {
		t.Fatalf("failed to build instance: %v", err)
	}

	// The pre-existing .gz is served, not a newly generated one.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/style.css", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	inst.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ce := w.Header().Get("Content-Encoding"); ce != "gzip" {
		t.Errorf("Content-Encoding = %q, want %q", ce, "gzip")
	}
	gr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	got, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}
	if string(got) != css {
		t.Errorf("decompressed body = %q, want %q", got, css)
	}
}
