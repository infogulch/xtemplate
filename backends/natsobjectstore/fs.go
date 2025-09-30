package natsobjectstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/spf13/afero"
)

var ErrNotSupported = errors.New("operation not supported on NATS Object Store filesystem")

// FS implements afero.Fs for NATS Object Store
type FS struct {
	store  jetstream.ObjectStore
	prefix string // optional prefix for all objects
}

// NewFS creates a new NATS Object Store filesystem
// Note: ctx parameter is ignored - we use context.Background() for NATS operations
// because the FS is shared across hot reloads and must outlive individual instance contexts
func NewFS(ctx context.Context, store jetstream.ObjectStore, prefix string) *FS {
	return &FS{
		store:  store,
		prefix: prefix,
	}
}

// Name returns the name of the filesystem
func (n *FS) Name() string {
	return "NatsObjectStoreFs"
}

// normalizePath converts filesystem path to object store key
func (n *FS) normalizePath(name string) string {
	name = path.Clean(name)
	// Handle root directory
	if name == "." {
		name = ""
	}
	if n.prefix != "" && name != "" {
		name = path.Join(n.prefix, name)
	} else if n.prefix != "" {
		name = n.prefix
	}
	return strings.TrimPrefix(name, "/")
}

// Open opens a file for reading
func (n *FS) Open(name string) (afero.File, error) {
	key := n.normalizePath(name)

	// Special case: root directory always exists
	if key == "" {
		return &dir{
			fs:   n,
			name: name,
			key:  key,
		}, nil
	}

	// Try to get as a file first
	// Use context.Background() because this FS is shared across hot reloads
	result, err := n.store.Get(context.Background(), key)
	if err == nil {
		// It's a file
		info, err := result.Info()
		if err != nil {
			return nil, err
		}

		// Read entire object into memory
		data, err := io.ReadAll(result)
		if err != nil {
			return nil, err
		}

		return &file{
			name:   name,
			data:   data,
			reader: bytes.NewReader(data),
			info:   info,
		}, nil
	}

	// Not a file, check if it's a directory by listing objects with this prefix
	prefix := key
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	// List all objects to check for children
	// Use context.Background() because this FS is shared across hot reloads
	infos, err := n.store.List(context.Background())
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}

	// Check if any objects start with this prefix
	hasChildren := false
	for _, info := range infos {
		if info != nil && strings.HasPrefix(info.Name, prefix) {
			hasChildren = true
			break
		}
	}

	if !hasChildren {
		return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrNotExist}
	}

	// It's a directory
	return &dir{
		fs:   n,
		name: name,
		key:  key,
	}, nil
}

// Stat returns FileInfo for the given path
func (n *FS) Stat(name string) (os.FileInfo, error) {
	key := n.normalizePath(name)

	// Special case: root directory always exists
	if key == "" {
		return &fileInfo{
			name:  ".",
			isDir: true,
		}, nil
	}

	// Try to get object info
	// Use context.Background() because this FS is shared across hot reloads
	info, err := n.store.GetInfo(context.Background(), key)
	if err == nil {
		// It's a file
		return &fileInfo{
			name:    path.Base(name),
			size:    int64(info.Size),
			modTime: info.ModTime,
			isDir:   false,
		}, nil
	}

	// Not a file, check if it's a directory
	prefix := key
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	// Use context.Background() because this FS is shared across hot reloads
	infos, err := n.store.List(context.Background())
	if err != nil {
		return nil, &os.PathError{Op: "stat", Path: name, Err: err}
	}

	for _, info := range infos {
		if info != nil && strings.HasPrefix(info.Name, prefix) {
			// It's a directory
			return &fileInfo{
				name:  path.Base(name),
				isDir: true,
			}, nil
		}
	}

	return nil, &os.PathError{Op: "stat", Path: name, Err: os.ErrNotExist}
}

func (n *FS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if flag == os.O_RDONLY {
		return n.Open(name)
	}
	return nil, ErrNotSupported
}

// Stub implementations for unsupported write operations
func (n *FS) Create(name string) (afero.File, error)                         { return nil, ErrNotSupported }
func (n *FS) Mkdir(name string, perm os.FileMode) error                      { return ErrNotSupported }
func (n *FS) MkdirAll(path string, perm os.FileMode) error                   { return ErrNotSupported }
func (n *FS) Remove(name string) error                                       { return ErrNotSupported }
func (n *FS) RemoveAll(path string) error                                    { return ErrNotSupported }
func (n *FS) Rename(oldname, newname string) error                           { return ErrNotSupported }
func (n *FS) Chmod(name string, mode os.FileMode) error                      { return ErrNotSupported }
func (n *FS) Chown(name string, uid, gid int) error                          { return ErrNotSupported }
func (n *FS) Chtimes(name string, atime time.Time, mtime time.Time) error   { return ErrNotSupported }


// file implements afero.File for regular files
type file struct {
	name   string
	data   []byte
	reader *bytes.Reader
	info   *jetstream.ObjectInfo
	closed bool
}

func (f *file) Close() error {
	f.closed = true
	return nil
}

func (f *file) Read(p []byte) (n int, err error) {
	if f.closed {
		return 0, afero.ErrFileClosed
	}
	return f.reader.Read(p)
}

func (f *file) ReadAt(p []byte, off int64) (n int, err error) {
	if f.closed {
		return 0, afero.ErrFileClosed
	}
	return f.reader.ReadAt(p, off)
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	if f.closed {
		return 0, afero.ErrFileClosed
	}
	return f.reader.Seek(offset, whence)
}

func (f *file) Write(p []byte) (n int, err error)       { return 0, ErrNotSupported }
func (f *file) WriteAt(p []byte, off int64) (n int, err error) { return 0, ErrNotSupported }
func (f *file) Name() string                            { return f.name }
func (f *file) Readdir(count int) ([]os.FileInfo, error) { return nil, ErrNotSupported }
func (f *file) Readdirnames(n int) ([]string, error)    { return nil, ErrNotSupported }
func (f *file) Stat() (os.FileInfo, error) {
	return &fileInfo{
		name:    path.Base(f.name),
		size:    int64(f.info.Size),
		modTime: f.info.ModTime,
		isDir:   false,
	}, nil
}
func (f *file) Sync() error                  { return nil }
func (f *file) Truncate(size int64) error    { return ErrNotSupported }
func (f *file) WriteString(s string) (ret int, err error) { return 0, ErrNotSupported }

// dir implements afero.File for directories
type dir struct {
	fs   *FS
	name string
	key  string
}

func (d *dir) Close() error                                { return nil }
func (d *dir) Read(p []byte) (n int, err error)            { return 0, ErrNotSupported }
func (d *dir) ReadAt(p []byte, off int64) (n int, err error) { return 0, ErrNotSupported }
func (d *dir) Seek(offset int64, whence int) (int64, error) { return 0, ErrNotSupported }
func (d *dir) Write(p []byte) (n int, err error)           { return 0, ErrNotSupported }
func (d *dir) WriteAt(p []byte, off int64) (n int, err error) { return 0, ErrNotSupported }
func (d *dir) Name() string                                { return d.name }
func (d *dir) Sync() error                                 { return nil }
func (d *dir) Truncate(size int64) error                   { return ErrNotSupported }
func (d *dir) WriteString(s string) (ret int, err error)   { return 0, ErrNotSupported }

func (d *dir) Stat() (os.FileInfo, error) {
	return &fileInfo{
		name:  path.Base(d.name),
		isDir: true,
	}, nil
}

func (d *dir) Readdir(count int) ([]os.FileInfo, error) {
	names, err := d.Readdirnames(count)
	if err != nil {
		return nil, err
	}

	infos := make([]os.FileInfo, len(names))
	for i, name := range names {
		fullPath := path.Join(d.name, name)
		info, err := d.fs.Stat(fullPath)
		if err != nil {
			return nil, err
		}
		infos[i] = info
	}

	return infos, nil
}

func (d *dir) Readdirnames(n int) ([]string, error) {
	// List all objects in the store
	// Use context.Background() because this FS is shared across hot reloads
	infos, err := d.fs.store.List(context.Background())
	if err != nil {
		return nil, err
	}

	prefix := d.key
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	// Collect direct children only
	seen := make(map[string]bool)
	var names []string

	for _, info := range infos {
		if info == nil {
			continue
		}

		// Skip if not under this prefix
		if !strings.HasPrefix(info.Name, prefix) {
			continue
		}

		// Get the relative path
		rel := strings.TrimPrefix(info.Name, prefix)
		if rel == "" {
			continue
		}

		// Check if this is a direct child or nested
		parts := strings.Split(rel, "/")
		childName := parts[0]

		if !seen[childName] {
			seen[childName] = true
			names = append(names, childName)
		}
	}

	return names, nil
}

// fileInfo implements os.FileInfo
type fileInfo struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
}

func (fi *fileInfo) Name() string       { return fi.name }
func (fi *fileInfo) Size() int64        { return fi.size }
func (fi *fileInfo) Mode() os.FileMode {
	if fi.isDir {
		return os.ModeDir | 0755
	}
	return 0644
}
func (fi *fileInfo) ModTime() time.Time { return fi.modTime }
func (fi *fileInfo) IsDir() bool        { return fi.isDir }
func (fi *fileInfo) Sys() interface{}   { return nil }

