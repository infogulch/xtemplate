package providers

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"sync"

	"github.com/infogulch/xtemplate"
)

func init() {
	xtemplate.RegisterDot(&DotFSProvider{})
}

// WithFS creates an [xtemplate.ConfigOverride] that can be used with
// [xtemplate.Config.Server], [xtemplate.Config.Instance], or [xtemplate.Main]
// to add an fs dot provider to the config.
func WithFS(name string, fs fs.FS) xtemplate.ConfigOverride {
	if fs == nil {
		panic(fmt.Sprintf("cannot create DotFSProvider with null FS with name %s", name))
	}
	return xtemplate.WithProvider(name, &DotFSProvider{FS: fs})
}

// DotFSProvider can configure an xtemplate dot field to provide file system
// access to templates. You can configure xtemplate to use it three ways:
//
// By setting a cli flag: “
type DotFSProvider struct {
	fs.FS `json:"-"`
	Path  string `json:"path"`
}

var _ encoding.TextMarshaler = &DotFSProvider{}

func (fs *DotFSProvider) MarshalText() ([]byte, error) {
	if fs.Path == "" {
		return nil, fmt.Errorf("FSDir cannot be marhsaled")
	}
	return []byte(fs.Path), nil
}

var _ encoding.TextUnmarshaler = &DotFSProvider{}

func (fs *DotFSProvider) UnmarshalText(b []byte) error {
	dir := string(b)
	if dir == "" {
		return fmt.Errorf("fs dir cannot be empty string")
	}
	fs.FS = os.DirFS(dir)
	return nil
}

var _ json.Marshaler = &DotFSProvider{}

func (d *DotFSProvider) MarshalJSON() ([]byte, error) {
	type T DotFSProvider
	return json.Marshal((*T)(d))
}

var _ json.Unmarshaler = &DotFSProvider{}

func (d *DotFSProvider) UnmarshalJSON(b []byte) error {
	type T DotFSProvider
	return json.Unmarshal(b, (*T)(d))
}

var _ xtemplate.CleanupDotProvider = &DotFSProvider{}

func (DotFSProvider) New() xtemplate.DotProvider { return &DotFSProvider{} }
func (DotFSProvider) Type() string               { return "fs" }
func (p *DotFSProvider) Value(r xtemplate.Request) (any, error) {
	if p.FS == nil {
		newfs := os.DirFS(p.Path)
		if _, err := newfs.(interface {
			Stat(string) (fs.FileInfo, error)
		}).Stat("."); err != nil {
			return &DotFS{}, fmt.Errorf("failed to stat fs current directory '%s': %w", p.Path, err)
		}
		p.FS = newfs
	}
	return &DotFS{p.FS, xtemplate.GetCtxLogger(r.R), r.W, r.R, make(map[fs.File]struct{})}, nil
}
func (p *DotFSProvider) Cleanup(a any, err error) error {
	v := a.(*DotFS)
	errs := []error{}
	for file := range v.opened {
		if err := file.Close(); err != nil {
			p := &fs.PathError{}
			if errors.As(err, &p) && p.Op == "close" && p.Err.Error() == "file already closed" {
				// ignore
			} else {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) != 0 {
		v.log.Warn("failed to close files", slog.Any("errors", errors.Join(errs...)))
	}
	return err
}

// DotFS is used to create an xtemplate dot field value that can access files in
// a local directory, or any [fs.FS].
type DotFS struct {
	fs     fs.FS
	log    *slog.Logger
	w      http.ResponseWriter
	r      *http.Request
	opened map[fs.File]struct{}
}

var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// List reads and returns a slice of names from the given directory
// relative to the FS root.
func (c *DotFS) List(name string) ([]string, error) {
	entries, err := fs.ReadDir(c.fs, path.Clean(name))
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, dirEntry := range entries {
		names = append(names, dirEntry.Name())
	}

	return names, nil
}

// Exists returns true if filename can be opened successfully.
func (c *DotFS) Exists(filename string) (bool, error) {
	file, err := c.fs.Open(filename)
	if err == nil {
		file.Close()
		return true, nil
	}
	return false, nil
}

// Stat returns Stat of a filename.
//
// Note: if you intend to read the file, afterwards, calling .Open instead may
// be more efficient.
func (c *DotFS) Stat(filename string) (fs.FileInfo, error) {
	filename = path.Clean(filename)
	file, err := c.fs.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return file.Stat()
}

// Read returns the contents of a filename relative to the FS root as a
// string.
func (c *DotFS) Read(filename string) (string, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	filename = path.Clean(filename)
	file, err := c.fs.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = io.Copy(buf, file)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// Open opens the file
func (c *DotFS) Open(path_ string) (fs.File, error) {
	path_ = path.Clean(path_)

	file, err := c.fs.Open(path_)
	if err != nil {
		return nil, fmt.Errorf("failed to open file at path '%s': %w", path_, err)
	}

	c.log.Debug("opened file", slog.String("path", path_))
	c.opened[file] = struct{}{}

	return file, nil
}
