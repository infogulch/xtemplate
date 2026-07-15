package dotfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/infogulch/xtemplate"
	"github.com/spf13/afero"
)

// UploadResult is returned by [DotFsRW.ReceiveFiles] for template use
// (redirect, DB insert) without reading upload.json.
type UploadResult struct {
	Dir   string
	Files []UploadedFile
}

// UploadedFile describes one stored file part from a multipart upload.
type UploadedFile struct {
	FormName     string
	OriginalName string
	StoredName   string
	Size         int64
	ContentType  string
}

// uploadManifest is written as upload.json under the request directory.
// Non-file form fields are intentionally omitted (may contain secrets).
type uploadManifest struct {
	Version    int                  `json:"version"`
	ReceivedAt string               `json:"received_at"`
	RequestID  string               `json:"request_id"`
	Dir        string               `json:"dir"`
	Limits     uploadManifestLimits `json:"limits"`
	TotalBytes int64                `json:"total_bytes"`
	Files      []uploadManifestFile `json:"files"`
}

type uploadManifestLimits struct {
	MaxFiles int   `json:"max_files"`
	MaxBytes int64 `json:"max_bytes"`
}

type uploadManifestFile struct {
	Index        int    `json:"index"`
	FormName     string `json:"form_name"`
	OriginalName string `json:"original_name"`
	StoredName   string `json:"stored_name"`
	Size         int64  `json:"size"`
	ContentType  string `json:"content_type"`
}

// ReceiveFiles streams a multipart/form-data body into dir/<request-id>/,
// stores file parts under sequential basenames, seeds non-file fields onto
// the request for later .Req.FormValue use, and writes upload.json last.
//
// See the package doc for limits, layout, extension rules, call order, and
// security notes. Template signature: ReceiveFiles dir maxFiles maxBytes → UploadResult.
func (d DotFsRW) ReceiveFiles(dir string, maxFiles, maxBytes int) (UploadResult, error) {
	var (
		nFiles     int
		totalBytes int64
	)

	ctx := context.Background()
	if d.r != nil {
		ctx = d.r.Context()
	}

	fail := func(err error) (UploadResult, error) {
		attrs := []slog.Attr{
			slog.String("dir", dir),
			slog.Int("files", nFiles),
			slog.Int64("total_bytes", totalBytes),
			slog.Any("error", err),
		}
		var es xtemplate.ErrorStatus
		if errors.As(err, &es) {
			attrs = append(attrs, slog.Int("status", int(es)))
		}
		d.log.LogAttrs(ctx, slog.LevelDebug, "ReceiveFiles failed", attrs...)
		return UploadResult{}, err
	}

	if d.r == nil {
		return fail(fmt.Errorf("ReceiveFiles: no request available"))
	}
	r := d.r
	w := d.w

	if d.received == nil {
		return fail(fmt.Errorf("ReceiveFiles: missing request state"))
	}
	if *d.received {
		return fail(fmt.Errorf("ReceiveFiles: already called for this request"))
	}
	if r.MultipartForm != nil {
		return fail(fmt.Errorf("ReceiveFiles: multipart form already parsed; call ReceiveFiles before FormValue/ParseMultipartForm"))
	}

	if maxFiles < 0 {
		return fail(fmt.Errorf("ReceiveFiles: maxFiles must be >= 0"))
	}
	if maxBytes < 0 {
		return fail(fmt.Errorf("ReceiveFiles: maxBytes must be >= 0"))
	}

	cleanDir, err := cleanUploadDir(dir)
	if err != nil {
		return fail(err)
	}

	rid := xtemplate.GetRequestId(r.Context())
	if rid == "" {
		return fail(fmt.Errorf("ReceiveFiles: failed to get request id: %w", err))
	}

	reqDir := path.Join(cleanDir, rid)
	if _, err := d.fs.Stat(reqDir); err == nil {
		return fail(fmt.Errorf("ReceiveFiles: request directory %q already exists", reqDir))
	} else if !isNotExist(err) {
		return fail(fmt.Errorf("ReceiveFiles: stat request dir: %w", err))
	}

	// Mark received early so a second call on the same request fails even if
	// the first attempt errors after this point. Pointer so Chroot shares it.
	*d.received = true

	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))
	mr, err := r.MultipartReader()
	if err != nil {
		return fail(clientErr(fmt.Errorf("ReceiveFiles: not a multipart request: %w", err)))
	}

	if err := d.fs.MkdirAll(reqDir, 0o755); err != nil {
		return fail(fmt.Errorf("ReceiveFiles: mkdir %q: %w", reqDir, err))
	}

	success := false
	defer func() {
		if success {
			return
		}
		if remErr := d.fs.RemoveAll(reqDir); remErr != nil {
			d.log.Warn("failed to clean up upload request dir",
				slog.String("dir", reqDir),
				slog.Any("error", remErr),
			)
		}
	}()

	ensureForm(r)

	pad := len(strconv.Itoa(maxFiles))
	if pad < 0 {
		return fail(fmt.Errorf("ReceiveFiles: invalid maxFiles %d", maxFiles))
	}

	var files []UploadedFile
	var manifestFiles []uploadManifestFile

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(mapBodyErr(fmt.Errorf("ReceiveFiles: read part: %w", err)))
		}

		filename := part.FileName()
		formName := part.FormName()

		// Empty client filename: treat as non-file field (do not store).
		if filename == "" {
			val, err := readPartString(part, int64(maxBytes))
			_ = part.Close()
			if err != nil {
				return fail(mapBodyErr(fmt.Errorf("ReceiveFiles: read field %q: %w", formName, err)))
			}
			seedFormField(r, formName, val)
			continue
		}

		if len(files) >= maxFiles {
			_ = part.Close()
			return fail(clientErr(fmt.Errorf("ReceiveFiles: too many files (max %d)", maxFiles)))
		}

		ext := storedExtension(filename)
		idx := len(files)
		storedName := fmt.Sprintf("%0*d%s", pad, idx, ext)
		outPath := path.Join(reqDir, storedName)
		ct := part.Header.Get("Content-Type")

		out, err := d.fs.Create(outPath)
		if err != nil {
			_ = part.Close()
			return fail(fmt.Errorf("ReceiveFiles: create %q: %w", outPath, err))
		}
		n, copyErr := io.Copy(out, part)
		closeErr := out.Close()
		_ = part.Close()
		if copyErr != nil {
			return fail(mapBodyErr(fmt.Errorf("ReceiveFiles: write %q: %w", outPath, copyErr)))
		}
		if closeErr != nil {
			return fail(fmt.Errorf("ReceiveFiles: close %q: %w", outPath, closeErr))
		}

		uf := UploadedFile{
			FormName:     formName,
			OriginalName: filename,
			StoredName:   storedName,
			Size:         n,
			ContentType:  ct,
		}
		files = append(files, uf)
		manifestFiles = append(manifestFiles, uploadManifestFile{
			Index:        idx,
			FormName:     formName,
			OriginalName: filename,
			StoredName:   storedName,
			Size:         n,
			ContentType:  ct,
		})
		totalBytes += n
		nFiles = len(files)
	}

	manifest := uploadManifest{
		Version:    1,
		ReceivedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		RequestID:  rid,
		Dir:        reqDir,
		Limits: uploadManifestLimits{
			MaxFiles: maxFiles,
			MaxBytes: int64(maxBytes),
		},
		TotalBytes: totalBytes,
		Files:      manifestFiles,
	}
	if err := writeUploadJSON(d.fs, reqDir, manifest); err != nil {
		return fail(err)
	}

	success = true
	result := UploadResult{Dir: reqDir, Files: files}

	d.log.LogAttrs(ctx, slog.LevelDebug, "ReceiveFiles ok",
		slog.String("dir", reqDir),
		slog.Int("files", nFiles),
		slog.Int64("total_bytes", totalBytes),
	)
	return result, nil
}

func cleanUploadDir(dir string) (string, error) {
	if dir == "" {
		return "", clientErr(fmt.Errorf("ReceiveFiles: dir must not be empty"))
	}
	cleaned := path.Clean(dir)
	if path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", clientErr(fmt.Errorf("ReceiveFiles: dir must be a relative path without '..': %q", dir))
	}
	// Reject volume/drive paths and backslashes.
	if len(cleaned) >= 2 && cleaned[1] == ':' {
		return "", clientErr(fmt.Errorf("ReceiveFiles: dir must be a relative path: %q", dir))
	}
	if strings.Contains(dir, "\\") || strings.Contains(cleaned, "\\") {
		return "", clientErr(fmt.Errorf("ReceiveFiles: dir must use '/' separators: %q", dir))
	}
	// path.Clean can leave "foo/.." as cleaned forms; after Clean, ".." segments
	// should only remain as prefix we already rejected. Also reject absolute
	// after Clean of paths like "/abs".
	if strings.Contains(cleaned, "/../") || strings.HasSuffix(cleaned, "/..") {
		return "", clientErr(fmt.Errorf("ReceiveFiles: dir must be a relative path without '..': %q", dir))
	}
	return cleaned, nil
}

// storedExtension returns a leading-dot extension for storage, or ".upload".
// Rules: path.Ext of the basename; body length 1–20; charset [a-zA-Z0-9];
// lowercase when storing.
func storedExtension(originalName string) string {
	base := path.Base(strings.ReplaceAll(originalName, "\\", "/"))
	ext := path.Ext(base)
	if ext == "" || ext == "." {
		return ".upload"
	}
	body := ext[1:]
	if len(body) < 1 || len(body) > 20 {
		return ".upload"
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return ".upload"
		}
	}
	return "." + strings.ToLower(body)
}

func ensureForm(r *http.Request) {
	if r.PostForm == nil {
		r.PostForm = make(url.Values)
	}
	if r.Form == nil {
		r.Form = make(url.Values)
		for k, vs := range r.URL.Query() {
			r.Form[k] = append([]string{}, vs...)
		}
	}
	if r.MultipartForm == nil {
		r.MultipartForm = &multipart.Form{
			Value: make(map[string][]string),
			File:  make(map[string][]*multipart.FileHeader),
		}
	} else {
		if r.MultipartForm.Value == nil {
			r.MultipartForm.Value = make(map[string][]string)
		}
		if r.MultipartForm.File == nil {
			r.MultipartForm.File = make(map[string][]*multipart.FileHeader)
		}
	}
}

func seedFormField(r *http.Request, name, value string) {
	r.PostForm.Add(name, value)
	r.Form.Add(name, value)
	r.MultipartForm.Value[name] = append(r.MultipartForm.Value[name], value)
}

func readPartString(part *multipart.Part, max int64) (string, error) {
	var r io.Reader = part
	if max >= 0 {
		r = io.LimitReader(part, max+1)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	if max >= 0 && int64(len(b)) > max {
		return "", &http.MaxBytesError{Limit: max}
	}
	return string(b), nil
}

func writeUploadJSON(afs afero.Fs, reqDir string, manifest uploadManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("ReceiveFiles: marshal upload.json: %w", err)
	}
	tmp := path.Join(reqDir, "upload.json.tmp")
	final := path.Join(reqDir, "upload.json")
	if err := afero.WriteFile(afs, tmp, data, 0o644); err != nil {
		return fmt.Errorf("ReceiveFiles: write upload.json.tmp: %w", err)
	}
	if err := afs.Rename(tmp, final); err != nil {
		_ = afs.Remove(tmp)
		if err2 := afero.WriteFile(afs, final, data, 0o644); err2 != nil {
			return fmt.Errorf("ReceiveFiles: write upload.json: %w", err2)
		}
	}
	return nil
}

func clientErr(err error) error {
	return fmt.Errorf("%w: %w", xtemplate.ErrorStatus(http.StatusBadRequest), err)
}

func mapBodyErr(err error) error {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return clientErr(fmt.Errorf("body exceeds maxBytes limit: %w", err))
	}
	return err
}

func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, afero.ErrFileNotFound)
}
