package providers

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path"
	"sync"
)

type dotFS struct {
	fs     fs.FS
	log    *slog.Logger
	opened map[fs.File]struct{}
}

// Dir
type Dir struct {
	dot  *dotFS
	path string
}

var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// Dir returns a
func (d Dir) Dir(name string) (Dir, error) {
	name = path.Clean(name)
	if st, err := d.Stat(name); err != nil {
		return Dir{}, err
	} else if !st.IsDir() {
		return Dir{}, fmt.Errorf("not a directory: %s", name)
	}
	return Dir{dot: d.dot, path: path.Join(d.path, name)}, nil
}

// List reads and returns a slice of names from the given directory relative to
// the FS root.
func (d Dir) List(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(d.dot.fs, path.Join(d.path, path.Clean(name)))
}

// Exists returns true if filename can be opened successfully.
func (d Dir) Exists(name string) bool {
	name = path.Join(d.path, path.Clean(name))
	file, err := d.Open(name)
	if err == nil {
		file.Close()
		return true
	}
	return false
}

// Stat returns Stat of a filename.
//
// Note: if you intend to read the file, afterwards, calling .Open instead may
// be more efficient.
func (d Dir) Stat(name string) (fs.FileInfo, error) {
	name = path.Join(d.path, path.Clean(name))
	file, err := d.dot.fs.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.Stat()
}

// Read returns the contents of a filename relative to the FS root as a string.
func (d Dir) Read(name string) (string, error) {
	name = path.Join(d.path, path.Clean(name))

	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	file, err := d.dot.fs.Open(name)
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
func (d Dir) Open(name string) (fs.File, error) {
	name = path.Join(d.path, path.Clean(name))

	file, err := d.dot.fs.Open(name)
	if err != nil {
		return nil, fmt.Errorf("failed to open file at path '%s': %w", name, err)
	}

	d.dot.log.Debug("opened file", slog.String("path", name))
	d.dot.opened[file] = struct{}{}

	return d.dot.fs.Open(name)
}
