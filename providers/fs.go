package providers

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"sync"
)

// DotFS is used to create an xtemplate dot field value that can access files in
// a local directory, or any [fs.FS].
//
// All public methods on DotFS are
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
