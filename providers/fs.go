package providers

import (
	"bytes"
	"encoding"
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

func WithFS(name string, fs fs.FS) xtemplate.ConfigOverride {
	if fs == nil {
		panic(fmt.Sprintf("cannot create DotFSProvider with null FS with name %s", name))
	}
	return xtemplate.WithProvider(name, DotFSProvider{FS: fs})
}

type DotFSProvider struct {
	fs.FS
	dir string
}

func (DotFSProvider) New() xtemplate.DotProvider { return &DotFSProvider{} }
func (DotFSProvider) Name() string               { return "fs" }

func (fs *DotFSProvider) UnmarshalText(b []byte) error {
	dir := string(b)
	if dir == "" {
		return fmt.Errorf("fs dir cannot be empty string")
	}
	fs.FS = os.DirFS(dir)
	return nil
}

func (fs *DotFSProvider) MarshalText() ([]byte, error) {
	if fs.dir == "" {
		return nil, fmt.Errorf("FSDir cannot be marhsaled")
	}
	return []byte(fs.dir), nil
}

var _ encoding.TextUnmarshaler = &DotFSProvider{}
var _ encoding.TextMarshaler = &DotFSProvider{}

func (fs DotFSProvider) Value(r xtemplate.Request) (any, error) {
	return &DotFS{fs, xtemplate.GetCtxLogger(r.R), r.W, r.R}, nil
}

// DotFS is used to create a dot field value that can access files in a local
// directory, or any [fs.FS].
type DotFS struct {
	fs  fs.FS
	log *slog.Logger
	w   http.ResponseWriter
	r   *http.Request
}

var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// ReadFile returns the contents of a filename relative to the FS root as a
// string.
func (c *DotFS) ReadFile(filename string) (string, error) {
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

// StatFile returns Stat of a filename
func (c *DotFS) StatFile(filename string) (fs.FileInfo, error) {
	filename = path.Clean(filename)
	file, err := c.fs.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return file.Stat()
}

// ListFiles reads and returns a slice of names from the given directory
// relative to the FS root.
func (c *DotFS) ListFiles(name string) ([]string, error) {
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

// FileExists returns true if filename can be opened successfully.
func (c *DotFS) FileExists(filename string) (bool, error) {
	file, err := c.fs.Open(filename)
	if err == nil {
		file.Close()
		return true, nil
	}
	return false, nil
}

// ServeFile aborts execution of the template and instead responds to the
// request with the content of the file at path_
func (c *DotFS) ServeFile(path_ string) (string, error) {
	path_ = path.Clean(path_)

	c.log.Debug("serving file response", slog.String("path", path_))

	file, err := c.fs.Open(path_)
	if err != nil {
		return "", fmt.Errorf("failed to open file at path '%s': %w", path_, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		c.log.Debug("error getting stat of file", slog.Any("error", err), slog.String("path", path_))
	}

	// TODO: Handle setting headers.

	http.ServeContent(c.w, c.r, path_, stat.ModTime(), file.(io.ReadSeeker))

	return "", xtemplate.ReturnError{}
}
