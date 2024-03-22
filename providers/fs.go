package providers

import (
	"bytes"
	"context"
	"encoding"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"reflect"
	"sync"

	"github.com/infogulch/xtemplate"
)

func init() {
	xtemplate.RegisterDot(&FSDot{})
}

func WithFS(name string, fs fs.FS) xtemplate.ConfigOverride {
	return xtemplate.WithProvider(name, FSDot{FS: fs})
}

type FSDot struct {
	fs.FS
	dir string
}

func (FSDot) New() xtemplate.DotProvider { return &FSDot{} }
func (FSDot) Name() string               { return "fs" }
func (FSDot) Type() reflect.Type         { return reflect.TypeOf(fsDot{}) }

func (fs *FSDot) UnmarshalText(b []byte) error {
	dir := string(b)
	if dir == "" {
		return fmt.Errorf("fs dir cannot be empty string")
	}
	fs.FS = os.DirFS(dir)
	return nil
}

func (fs *FSDot) MarshalText() ([]byte, error) {
	if fs.dir == "" {
		return nil, fmt.Errorf("FSDir cannot be marhsaled")
	}
	return []byte(fs.dir), nil
}

var _ encoding.TextUnmarshaler = &FSDot{}
var _ encoding.TextMarshaler = &FSDot{}

func (fs FSDot) Value(log *slog.Logger, sctx context.Context, w http.ResponseWriter, r *http.Request) (reflect.Value, error) {
	return reflect.ValueOf(fsDot{fs, log}), nil
}

type fsDot struct {
	fs  fs.FS
	log *slog.Logger
}

var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// ReadFile returns the contents of a filename relative to the site root.
// Note that included files are NOT escaped, so you should only include
// trusted files. If it is not trusted, be sure to use escaping functions
// in your template.
func (c *fsDot) ReadFile(filename string) (string, error) {
	if c.fs == nil {
		return "", fmt.Errorf("context file system is not configured")
	}
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
func (c *fsDot) StatFile(filename string) (fs.FileInfo, error) {
	if c.fs == nil {
		return nil, fmt.Errorf("context file system is not configured")
	}
	filename = path.Clean(filename)
	file, err := c.fs.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return file.Stat()
}

// ListFiles reads and returns a slice of names from the given
// directory relative to the root of c.
func (c *fsDot) ListFiles(name string) ([]string, error) {
	if c.fs == nil {
		return nil, fmt.Errorf("context file system is not configured")
	}
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
func (c *fsDot) FileExists(filename string) (bool, error) {
	if c.fs == nil {
		return false, fmt.Errorf("context file system is not configured")
	}
	file, err := c.fs.Open(filename)
	if err == nil {
		file.Close()
		return true, nil
	}
	return false, nil
}

// ServeFile aborts execution of the template and instead responds to the
// request with the content of the file at path_
func (c *fsDot) ServeFile(path_ string) (string, error) {
	return "", xtemplate.NewHandlerError("ServeFile", func(w http.ResponseWriter, r *http.Request) {
		path_ = path.Clean(path_)

		c.log.Debug("serving file response", slog.String("path", path_))

		file, err := c.fs.Open(path_)
		if err != nil {
			c.log.Debug("failed to open file", slog.Any("error", err), slog.String("path", path_))
			http.Error(w, "internal server error", 500)
			return
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			c.log.Debug("error getting stat of file", slog.Any("error", err), slog.String("path", path_))
		}

		http.ServeContent(w, r, path_, stat.ModTime(), file.(io.ReadSeeker))
	})
}
