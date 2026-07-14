package dotfs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/infogulch/xtemplate"
	"github.com/infogulch/xtemplate/providers/dotfs"
)

func newMemFS(t *testing.T, files map[string]string) afero.Fs {
	t.Helper()
	fs := afero.NewMemMapFs()
	for name, content := range files {
		if err := afero.WriteFile(fs, name, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %q to mem fs: %v", name, err)
		}
	}
	return fs
}

func buildInstance(t *testing.T, files map[string]string, opts ...xtemplate.Option) *xtemplate.Instance {
	t.Helper()
	fs := newMemFS(t, files)
	cfg := xtemplate.New()
	allOpts := append([]xtemplate.Option{xtemplate.WithTemplateFS(fs)}, opts...)
	inst, _, _, err := cfg.Instance(allOpts...)
	if err != nil {
		t.Fatalf("failed to build instance: %v", err)
	}
	return inst
}

func doRequest(inst *xtemplate.Instance, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, target, nil)
	inst.ServeHTTP(w, r)
	return w
}

func TestDirProvider(t *testing.T) {
	dataFS := newMemFS(t, map[string]string{
		"message.txt": "FILE-CONTENT",
	})
	inst := buildInstance(t,
		map[string]string{
			"read.html": `{{.Files.Read "message.txt"}}`,
		},
		dotfs.WithFs("Files", dataFS),
	)

	w := doRequest(inst, http.MethodGet, "/read")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); !strings.Contains(body, "FILE-CONTENT") {
		t.Errorf("body = %q, want it to contain %q", body, "FILE-CONTENT")
	}
}

func TestInit_WritableProbeSucceedsAndNoLeftover(t *testing.T) {
	mem := afero.NewMemMapFs()
	cfg := &dotfs.DotFsConfig{Name: "FS", FS: mem, Writable: true}
	if err := cfg.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	entries, err := afero.ReadDir(mem, ".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".xtemplate-write-probe-") {
			t.Errorf("probe file left behind: %s", e.Name())
		}
	}
}

func TestInit_WritableWithReadOnlyFsFails(t *testing.T) {
	mem := afero.NewMemMapFs()
	ro := afero.NewReadOnlyFs(mem)
	cfg := &dotfs.DotFsConfig{Name: "FS", FS: ro, Writable: true}
	err := cfg.Init(context.Background())
	if err == nil {
		t.Fatal("Init: expected error for ReadOnlyFs with writable:true")
	}
	if !strings.Contains(err.Error(), "ReadOnlyFs") && !strings.Contains(err.Error(), "not writable") {
		t.Errorf("Init error = %v, want mention of read-only/not writable", err)
	}
}

func TestInit_NonWritableWrapsReadOnly(t *testing.T) {
	mem := afero.NewMemMapFs()
	_ = afero.WriteFile(mem, "a.txt", []byte("x"), 0644)
	cfg := &dotfs.DotFsConfig{Name: "FS", FS: mem, Writable: false}
	if err := cfg.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, ok := cfg.FS.(*afero.ReadOnlyFs); !ok {
		t.Fatalf("FS type = %T, want *afero.ReadOnlyFs", cfg.FS)
	}
	if err := afero.WriteFile(cfg.FS, "b.txt", []byte("y"), 0644); err == nil {
		t.Fatal("expected write to fail on read-only wrapped FS")
	}
}

func TestWritableFalse_ReceiveFilesUnknownField(t *testing.T) {
	dataFS := newMemFS(t, nil)
	inst := buildInstance(t,
		map[string]string{
			"routes.html": `{{define "POST /upload"}}{{.FS.ReceiveFiles "uploads" 10 1000}}{{end}}`,
		},
		dotfs.WithFs("FS", dataFS),
	)

	body, ctype := multipartBody(t, map[string]string{"title": "hi"}, nil)
	w := postUpload(inst, body, ctype)
	if w.Code == http.StatusOK {
		t.Fatalf("status = %d, want error (ReceiveFiles not on read-only DotFs)", w.Code)
	}
}

func TestReceiveFiles_HappyPath(t *testing.T) {
	dataFS := newMemFS(t, nil)
	inst := buildInstance(t,
		map[string]string{
			"routes.html": `{{define "POST /upload"}}
{{- $u := .FS.ReceiveFiles "uploads" 10 10485760 -}}
dir={{$u.Dir}}
files={{len $u.Files}}
title={{.Req.FormValue "title"}}
stored={{(index $u.Files 0).StoredName}}
orig={{(index $u.Files 0).OriginalName}}
{{end}}`,
		},
		dotfs.WithFsWritable("FS", dataFS),
	)

	body, ctype := multipartBody(t,
		map[string]string{"title": "My Title"},
		[]filePart{{field: "photo", name: "My Photo.JPG", data: []byte("jpeg-bytes")}},
	)
	w := postUpload(inst, body, ctype)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %q", w.Code, w.Body.String())
	}
	out := w.Body.String()
	if !strings.Contains(out, "title=My Title") {
		t.Errorf("body = %q, want FormValue title", out)
	}
	if !strings.Contains(out, "files=1") {
		t.Errorf("body = %q, want files=1", out)
	}
	if !strings.Contains(out, "stored=00.jpg") {
		t.Errorf("body = %q, want stored=00.jpg", out)
	}
	if !strings.Contains(out, "orig=My Photo.JPG") {
		t.Errorf("body = %q, want original name", out)
	}

	rootEntries, err := afero.ReadDir(dataFS, "uploads")
	if err != nil {
		t.Fatalf("uploads dir: %v", err)
	}
	if len(rootEntries) != 1 {
		t.Fatalf("want 1 request dir under uploads, got %d", len(rootEntries))
	}
	reqDir := path.Join("uploads", rootEntries[0].Name())
	entries, err := afero.ReadDir(dataFS, reqDir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", reqDir, err)
	}
	foundUploadJSON := false
	foundFile := false
	for _, e := range entries {
		switch {
		case e.Name() == "upload.json":
			foundUploadJSON = true
			raw, err := afero.ReadFile(dataFS, path.Join(reqDir, "upload.json"))
			if err != nil {
				t.Fatalf("read upload.json: %v", err)
			}
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("upload.json: %v", err)
			}
			if _, ok := m["fields"]; ok {
				t.Error("upload.json must not contain fields key")
			}
			if m["version"].(float64) != 1 {
				t.Errorf("version = %v, want 1", m["version"])
			}
			files, ok := m["files"].([]any)
			if !ok || len(files) != 1 {
				t.Fatalf("files = %v", m["files"])
			}
			f0 := files[0].(map[string]any)
			if f0["original_name"] != "My Photo.JPG" {
				t.Errorf("original_name = %v", f0["original_name"])
			}
			if f0["stored_name"] != "00.jpg" {
				t.Errorf("stored_name = %v, want 00.jpg", f0["stored_name"])
			}
		case strings.HasSuffix(e.Name(), ".jpg"):
			foundFile = true
			data, _ := afero.ReadFile(dataFS, path.Join(reqDir, e.Name()))
			if string(data) != "jpeg-bytes" {
				t.Errorf("file content = %q", data)
			}
		}
	}
	if !foundUploadJSON {
		t.Error("missing upload.json")
	}
	if !foundFile {
		t.Error("missing stored image file")
	}
	if !strings.Contains(out, "dir="+reqDir) {
		t.Errorf("body dir want %q in %q", reqDir, out)
	}
}

func TestReceiveFiles_ExtensionPolicy(t *testing.T) {
	cases := []struct {
		name    string
		orig    string
		wantExt string
	}{
		{"normal", "photo.PNG", ".png"},
		{"empty", "noext", ".upload"},
		{"long", "x." + strings.Repeat("a", 21), ".upload"},
		{"nonascii", "x.jpeǵ", ".upload"},
		{"dots", "archive.tar.gz", ".gz"},
		{"badchars", "x.foo-bar", ".upload"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dataFS := newMemFS(t, nil)
			inst := buildInstance(t,
				map[string]string{
					"routes.html": `{{define "POST /upload"}}{{$u := .FS.ReceiveFiles "up" 5 10000}}{{(index $u.Files 0).StoredName}}{{end}}`,
				},
				dotfs.WithFsWritable("FS", dataFS),
			)
			body, ctype := multipartBody(t, nil, []filePart{{field: "f", name: tc.orig, data: []byte("d")}})
			w := postUpload(inst, body, ctype)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d body = %q", w.Code, w.Body.String())
			}
			got := strings.TrimSpace(w.Body.String())
			if !strings.HasSuffix(got, tc.wantExt) {
				t.Errorf("StoredName = %q, want suffix %q", got, tc.wantExt)
			}
		})
	}
}

func TestReceiveFiles_TooManyFiles(t *testing.T) {
	dataFS := newMemFS(t, nil)
	inst := buildInstance(t,
		map[string]string{
			"routes.html": `{{define "POST /upload"}}{{.FS.ReceiveFiles "up" 1 10000}}{{end}}`,
		},
		dotfs.WithFsWritable("FS", dataFS),
	)
	body, ctype := multipartBody(t, nil, []filePart{
		{field: "a", name: "a.txt", data: []byte("1")},
		{field: "b", name: "b.txt", data: []byte("2")},
	})
	w := postUpload(inst, body, ctype)
	if w.Code == http.StatusOK {
		t.Fatalf("expected error status, got 200 body=%q", w.Body.String())
	}
	if ents, _ := afero.ReadDir(dataFS, "up"); len(ents) != 0 {
		t.Errorf("expected cleaned up dirs, got %v", ents)
	}
}

func TestReceiveFiles_OversizeBody(t *testing.T) {
	dataFS := newMemFS(t, nil)
	inst := buildInstance(t,
		map[string]string{
			"routes.html": `{{define "POST /upload"}}{{.FS.ReceiveFiles "up" 10 32}}{{end}}`,
		},
		dotfs.WithFsWritable("FS", dataFS),
	)
	big := bytes.Repeat([]byte("x"), 1000)
	body, ctype := multipartBody(t, nil, []filePart{{field: "f", name: "big.bin", data: big}})
	w := postUpload(inst, body, ctype)
	if w.Code == http.StatusOK {
		t.Fatalf("expected error for oversize, got 200")
	}
}

func TestReceiveFiles_DoubleCall(t *testing.T) {
	dataFS := newMemFS(t, nil)
	inst := buildInstance(t,
		map[string]string{
			"routes.html": `{{define "POST /upload"}}
{{$u := .FS.ReceiveFiles "up" 10 10000}}
{{.FS.ReceiveFiles "up" 10 10000}}
{{end}}`,
		},
		dotfs.WithFsWritable("FS", dataFS),
	)
	body, ctype := multipartBody(t, map[string]string{"t": "1"}, []filePart{{field: "f", name: "a.txt", data: []byte("hi")}})
	w := postUpload(inst, body, ctype)
	if w.Code == http.StatusOK {
		t.Fatalf("expected error on double ReceiveFiles, got 200 body=%q", w.Body.String())
	}
}

func TestReceiveFiles_NonMultipart(t *testing.T) {
	dataFS := newMemFS(t, nil)
	inst := buildInstance(t,
		map[string]string{
			"routes.html": `{{define "POST /upload"}}{{.FS.ReceiveFiles "up" 10 10000}}{{end}}`,
		},
		dotfs.WithFsWritable("FS", dataFS),
	)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("x=y"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	inst.ServeHTTP(w, r)
	if w.Code == http.StatusOK {
		t.Fatalf("expected error for non-multipart, got 200")
	}
}

func TestReceiveFiles_MidStreamFailureCleansUp(t *testing.T) {
	base := afero.NewMemMapFs()
	// failAfter 2: Init write probe uses Create once; the next Create is the
	// stored upload file and should fail so the request dir is cleaned up.
	ffs := &failAfterNCreates{Fs: base, failAfter: 2}

	inst := buildInstance(t,
		map[string]string{
			"routes.html": `{{define "POST /upload"}}{{.FS.ReceiveFiles "up" 10 10000}}{{end}}`,
		},
		dotfs.WithFsWritable("FS", ffs),
	)
	body, ctype := multipartBody(t, nil, []filePart{{field: "f", name: "a.txt", data: []byte("hi")}})
	w := postUpload(inst, body, ctype)
	if w.Code == http.StatusOK {
		t.Fatalf("expected failure, got 200")
	}
	if ents, err := afero.ReadDir(base, "up"); err == nil && len(ents) != 0 {
		t.Errorf("expected cleanup of request dir, still have %v", ents)
	}
}

func postUpload(inst *xtemplate.Instance, body *bytes.Buffer, ctype string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/upload", body)
	r.Header.Set("Content-Type", ctype)
	inst.ServeHTTP(w, r)
	return w
}

type filePart struct {
	field string
	name  string
	data  []byte
}

func multipartBody(t *testing.T, fields map[string]string, files []filePart) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range files {
		pw, err := mw.CreateFormFile(f.field, f.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pw.Write(f.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

// failAfterNCreates wraps an afero.Fs and fails Create after N successful creates.
type failAfterNCreates struct {
	afero.Fs
	failAfter int
	creates   int
}

func (f *failAfterNCreates) Create(name string) (afero.File, error) {
	f.creates++
	if f.creates >= f.failAfter {
		return nil, io.ErrUnexpectedEOF
	}
	return f.Fs.Create(name)
}
