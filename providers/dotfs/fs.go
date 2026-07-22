// Package dotfs is the core filesystem [xtemplate] dot provider (type "fs").
//
// # Config
//
// JSON / provider config fields:
//
//   - name (required): dot field name, e.g. "FS" → {{.FS}}
//   - path: root directory on disk when FS is not injected in Go
//   - writable (bool, default false): when true, the per-request value is
//     [DotFsRW] (read methods plus [DotFsRW.ReceiveFiles]); when false, [DotFs]
//     only and the store is wrapped with [afero.NewReadOnlyFs]
//
// Go: [WithFs] (read-only) or [WithFsWritable]. Caddyfile: writable true inside
// provider fs … { }.
//
// When writable is true, [DotFsConfig.Init] probes that the provider root can
// create and delete a file; instance load fails otherwise. The root itself must
// be writable (not only a subdirectory). Put auth in front of open upload routes.
//
// # Read API ([DotFs] / embedded in [DotFsRW])
//
// Read, ReadDir, Open, Stat, Exists, ExistsDir, Chroot.
//
// # Upload API ([DotFsRW.ReceiveFiles])
//
// Template signature: ReceiveFiles dir maxFiles maxBytes → [UploadResult]
//
// All three arguments are required. Streams multipart/form-data (MultipartReader,
// not ParseMultipartForm) so large files are not fully buffered in memory.
//
//   - File parts (non-empty client filename) are stored under {dir}/{request-id}/
//     with sequential basenames (00.jpg, 01.upload, …)—never the client path.
//   - Non-file parts are seeded onto the request Form/PostForm/MultipartForm.Value
//     so {{.Req.FormValue}} works after ReceiveFiles. Call ReceiveFiles before
//     FormValue or ParseMultipartForm; a second call on the same request errors.
//   - upload.json is written last with file metadata only (no text form fields;
//     those may hold secrets). Original filenames appear in the manifest for
//     debugging and may themselves be sensitive.
//   - Stored extension: path.Ext of the original basename; body length 1–20 and
//     charset [a-zA-Z0-9], lowercased; otherwise .upload.
//   - dir must be a relative path without ".." under the provider root.
//   - On failure after mkdir, the request directory is removed best-effort.
//
// Success and failure are logged at debug on the request logger.
//
// Example:
//
//	{{- define "POST /upload"}}
//	{{$u := .FS.ReceiveFiles "uploads" 10 10485760}}
//	{{$title := .Req.FormValue "title"}}
//	{{.Resp.AddHeader "Location" (printf "/files/%s" $u.Dir)}}
//	{{.Resp.ReturnStatus 303}}
//	{{end}}
//
// Runnable demo (list, read, and upload): examples/filebrowser.
package dotfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/infogulch/xtemplate"
	"github.com/spf13/afero"
)

var bufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func init() {
	xtemplate.RegisterProvider("fs", func() xtemplate.Provider { return &DotFsConfig{} })
}

// WithFs creates an [xtemplate.Option] that can be used with
// [xtemplate.Config.Server], [xtemplate.Config.Instance], or [xtemplate.Main]
// to add a read-only fs dot provider to the config.
//
// The given FS is wrapped with [afero.NewReadOnlyFs] during Init. To enable
// uploads, use [WithFsWritable] or set [DotFsConfig.Writable].
func WithFs(name string, afs afero.Fs) xtemplate.Option {
	return withFs(name, afs, false)
}

// WithFsWritable is like [WithFs] but enables write/upload methods ([DotFsRW])
// and runs an Init-time writability probe on the given FS.
func WithFsWritable(name string, afs afero.Fs) xtemplate.Option {
	return withFs(name, afs, true)
}

func withFs(name string, afs afero.Fs, writable bool) xtemplate.Option {
	return func(c *xtemplate.Config) error {
		if afs == nil {
			return fmt.Errorf("cannot create DotFSProvider with null FS with name %s", name)
		}
		c.Providers = append(c.Providers, &DotFsConfig{Name: name, FS: afs, Writable: writable})
		return nil
	}
}

// DotFsConfig configures an xtemplate dot field to provide file system access
// to templates.
//
// When Writable is false (default), the template field is a [DotFs] with
// read-only methods and the backing FS is wrapped with [afero.NewReadOnlyFs].
// When Writable is true, the field is a [DotFsRW] that includes [DotFsRW.ReceiveFiles],
// and Init probes that the root can create and delete files. The provider root
// must be writable when Writable is true (not only a subdirectory).
type DotFsConfig struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Writable bool     `json:"writable"`
	FS       afero.Fs `json:"-"`
}

var (
	_ xtemplate.Initializer = &DotFsConfig{}
	_ xtemplate.Finalizer   = &DotFsConfig{}
)

func (c *DotFsConfig) FieldName() string { return c.Name }

func (p *DotFsConfig) Prototype() any {
	if p.Writable {
		return DotFsRW{}
	}
	return DotFs{}
}

func (p *DotFsConfig) Init(ctx context.Context) error {
	if p.FS == nil {
		newfs := afero.NewBasePathFs(afero.NewOsFs(), p.Path)
		if _, err := newfs.Stat("."); err != nil {
			return fmt.Errorf("failed to stat fs current directory '%s': %w", p.Path, err)
		}
		p.FS = newfs
	}

	if !p.Writable {
		if _, ok := p.FS.(*afero.ReadOnlyFs); !ok {
			p.FS = afero.NewReadOnlyFs(p.FS)
		}
		return nil
	}

	return probeWritable(p.FS, p.rootHint())
}

func (p *DotFsConfig) rootHint() string {
	if p.Path != "" {
		return p.Path
	}
	if p.Name != "" {
		return p.Name
	}
	return "(injected FS)"
}

// probeWritable verifies the backing FS can create and delete a file under the
// provider root. Called only from Init when writable is true.
func probeWritable(afs afero.Fs, pathHint string) error {
	if _, ok := afs.(*afero.ReadOnlyFs); ok {
		return fmt.Errorf("fs provider root %q is wrapped with ReadOnlyFs; writable: true requires a writable filesystem", pathHint)
	}

	name := ".xtemplate-write-probe-" + uuid.NewString()
	f, err := afs.Create(name)
	if err != nil {
		return fmt.Errorf("fs provider root %q is not writable (create probe failed): %w", pathHint, err)
	}
	// Best-effort remove on any exit path (including panic) so the probe file
	// does not linger when possible.
	removed := false
	defer func() {
		if !removed {
			_ = afs.Remove(name)
		}
	}()
	if err := f.Close(); err != nil {
		return fmt.Errorf("fs provider root %q is not writable (close probe failed): %w", pathHint, err)
	}
	if err := afs.Remove(name); err != nil {
		return fmt.Errorf("fs provider root %q is not writable (remove probe failed): %w", pathHint, err)
	}
	removed = true
	return nil
}

func (p *DotFsConfig) Value(w http.ResponseWriter, r *http.Request) (any, error) {
	base := DotFs{
		fs:     p.FS,
		log:    xtemplate.GetLogger(r.Context()),
		opened: make(map[afero.File]struct{}),
	}
	if p.Writable {
		received := false
		return DotFsRW{
			DotFs:    base,
			w:        w,
			r:        r,
			received: &received,
		}, nil
	}
	return base, nil
}

func (p *DotFsConfig) Finalize(a any, err error) error {
	var d DotFs
	switch v := a.(type) {
	case DotFs:
		d = v
	case DotFsRW:
		d = v.DotFs
	default:
		return err
	}

	errs := []error{}
	for file := range d.opened {
		if cerr := file.Close(); cerr != nil {
			pe := &fs.PathError{}
			if errors.As(cerr, &pe) && pe.Op == "close" && pe.Err.Error() == "file already closed" {
				// ignore
			} else {
				errs = append(errs, cerr)
			}
		}
	}
	if len(errs) != 0 {
		d.log.Warn("failed to close files", slog.Any("errors", errors.Join(errs...)))
	}
	return err
}

// DotFs provides read-only template access to a filesystem rooted at a
// configured path. Write/upload methods are not present; use [DotFsRW] via
// writable: true.
type DotFs struct {
	fs  afero.Fs
	log *slog.Logger
	// opened is written without synchronization. This is safe only because
	// template execution for a single request runs on a single goroutine, so
	// there are no concurrent writers to this map.
	opened map[afero.File]struct{}
}

// DotFsRW embeds [DotFs] for the shared read method set and adds request-scoped
// state for [DotFsRW.ReceiveFiles]. Returned when the fs provider is configured
// with writable: true.
type DotFsRW struct {
	DotFs
	w http.ResponseWriter
	r *http.Request
	// received is a pointer so Chroot copies share the one-call-per-request flag.
	received *bool
}

// Chroot returns a copy of the filesystem with root changed to path.
func (d DotFs) Chroot(path string) (DotFs, error) {
	if _, err := d.fs.Stat(path); err != nil {
		return DotFs{}, fmt.Errorf("failed to chroot to %#v: %w", path, err)
	}
	return DotFs{
		fs:     afero.NewBasePathFs(d.fs, path),
		log:    d.log,
		opened: d.opened,
	}, nil
}

// Chroot returns a copy of the filesystem with root changed to path. The
// result remains writable under the chroot and shares upload request state
// with the parent.
func (d DotFsRW) Chroot(path string) (DotFsRW, error) {
	base, err := d.DotFs.Chroot(path)
	if err != nil {
		return DotFsRW{}, err
	}
	return DotFsRW{
		DotFs:    base,
		w:        d.w,
		r:        d.r,
		received: d.received,
	}, nil
}

// ReadDir reads the directory named by dirname and returns a list of directory
// entries sorted by filename.
func (d DotFs) ReadDir(path string) ([]fs.FileInfo, error) {
	return afero.ReadDir(d.fs, path)
}

// Exists returns true if filename can be opened successfully.
func (d DotFs) Exists(filename string) bool {
	_, err := d.fs.Stat(filename)
	return err == nil
}

// ExistsDir returns true if dirname exists and is a directory.
func (d DotFs) ExistsDir(dirname string) bool {
	s, err := d.fs.Stat(dirname)
	return err == nil && s.IsDir()
}

// Stat returns Stat of a filename.
//
// Note: if you intend to read the file afterwards, calling .Open instead may
// be more efficient.
func (d DotFs) Stat(filename string) (fs.FileInfo, error) {
	return d.fs.Stat(filename)
}

// Read returns the contents of a filename relative to the FS root as a string.
func (d DotFs) Read(name string) (string, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	file, err := d.fs.Open(name)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	stat, err := file.Stat()
	if err != nil {
		return "", err
	}
	buf.Grow(int(stat.Size()))

	_, err = io.Copy(buf, file)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// Open opens the named file and tracks it for cleanup at end-of-request.
func (d DotFs) Open(filename string) (afero.File, error) {
	file, err := d.fs.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file at path %#v: %w", filename, err)
	}

	d.log.Debug("opened file", slog.String("filename", filename))
	d.opened[file] = struct{}{}

	return file, nil
}
