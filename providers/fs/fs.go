package fs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"sync"

	"github.com/infogulch/xtemplate"
	"github.com/spf13/afero"
)

var bufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func init() {
	xtemplate.Register("fs", func() xtemplate.DotConfig { return &DotFsConfig{} })
}

// WithFs creates an [xtemplate.Option] that can be used with
// [xtemplate.Config.Server], [xtemplate.Config.Instance], or [xtemplate.Main]
// to add an fs dot provider to the config.
func WithFs(name string, afs afero.Fs) xtemplate.Option {
	return func(c *xtemplate.Config) error {
		if afs == nil {
			return fmt.Errorf("cannot create DotFSProvider with null FS with name %s", name)
		}
		c.Providers = append(c.Providers, &DotFsConfig{Name: name, FS: afs})
		return nil
	}
}

// DotFsConfig configures an xtemplate dot field to provide file system access
// to templates.
type DotFsConfig struct {
	Name string   `json:"name"`
	Path string   `json:"path"`
	FS   afero.Fs `json:"-"`
}

var _ xtemplate.CleanupDotProvider = &DotFsConfig{}

func (c *DotFsConfig) FieldName() string { return c.Name }

func (p *DotFsConfig) Init(ctx context.Context) error {
	if p.FS != nil {
		return nil
	}
	newfs := afero.NewBasePathFs(afero.NewOsFs(), p.Path)
	if _, err := newfs.(interface {
		Stat(string) (fs.FileInfo, error)
	}).Stat("."); err != nil {
		return fmt.Errorf("failed to stat fs current directory '%s': %w", p.Path, err)
	}
	p.FS = newfs
	return nil
}

func (p *DotFsConfig) Value(r xtemplate.Request) (any, error) {
	return DotFs{p.FS, xtemplate.GetLogger(r.R.Context()), make(map[afero.File]struct{})}, nil
}

func (p *DotFsConfig) Cleanup(a any, err error) error {
	v := a.(DotFs)
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

// DotFs provides template access to a filesystem rooted at a configured path.
type DotFs struct {
	fs  afero.Fs
	log *slog.Logger
	// opened is written without synchronization. This is safe only because
	// template execution for a single request runs on a single goroutine, so
	// there are no concurrent writers to this map.
	opened map[afero.File]struct{}
}

// Chroot returns a copy of the filesystem with root changed to path.
func (d DotFs) Chroot(path string) (DotFs, error) {
	if _, err := d.fs.Stat(path); err != nil {
		return DotFs{}, fmt.Errorf("failed to chroot to %#v: %w", path, err)
	}
	afs := afero.NewBasePathFs(d.fs, path)
	return DotFs{
		fs:     afs,
		log:    d.log,
		opened: d.opened,
	}, nil
}

// ReadDir reads the directory named by dirname and returns a list of directory entries sorted by filename.
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
