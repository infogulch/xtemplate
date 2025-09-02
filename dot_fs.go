package xtemplate

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"log/slog"

	"github.com/spf13/afero"
)

// Dir
type Dir struct {
	fs     afero.Fs
	log    *slog.Logger
	opened map[afero.File]struct{}
}

// Chroot returns a copy of the filesystem with root changed to path.
func (d Dir) Chroot(path string) (Dir, error) {
	if _, err := d.fs.Stat(path); err != nil {
		return Dir{}, fmt.Errorf("failed to chroot to %#v: %w", path, err)
	}
	fs := afero.NewBasePathFs(d.fs, path)
	return Dir{
		fs:     fs,
		log:    d.log,
		opened: d.opened,
	}, nil
}

// ReadDir reads the directory named by dirname and returns a list of directory entries sorted by filename.
func (d Dir) ReadDir(path string) ([]fs.FileInfo, error) {
	return afero.ReadDir(d.fs, path)
}

// Exists returns true if filename can be opened successfully.
func (d Dir) Exists(filename string) bool {
	_, err := d.fs.Stat(filename)
	return err == nil
}

// Stat returns Stat of a filename.
//
// Note: if you intend to read the file, afterwards, calling .Open instead may
// be more efficient.
func (d Dir) Stat(filename string) (fs.FileInfo, error) {
	return d.fs.Stat(filename)
}

// Read returns the contents of a filename relative to the FS root as a string.
func (d Dir) Read(name string) (string, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)
	defer buf.Reset()

	file, err := d.fs.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()

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

// Open opens the file
func (d Dir) Open(filename string) (afero.File, error) {
	file, err := d.fs.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file at path %#v: %w", filename, err)
	}

	d.log.Debug("opened file", slog.String("filename", filename))
	d.opened[file] = struct{}{}

	return file, nil
}
