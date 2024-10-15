package xtemplate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
)

// WithDir creates an [xtemplate.Option] that can be used with
// [xtemplate.Config.Server], [xtemplate.Config.Instance], or [xtemplate.Main]
// to add an fs dot provider to the config.
func WithDir(name string, fs fs.FS) Option {
	return func(c *Config) error {
		if fs == nil {
			return fmt.Errorf("cannot create DotFSProvider with null FS with name %s", name)
		}
		c.Directories = append(c.Directories, DotDirConfig{Name: name, FS: fs})
		return nil
	}
}

// DotDirConfig can configure an xtemplate dot field to provide file system
// access to templates. You can configure xtemplate to use it three ways:
//
// By setting a cli flag: “
type DotDirConfig struct {
	Name  string `json:"name"`
	fs.FS `json:"-"`
	Path  string `json:"path"`
}

var _ CleanupDotProvider = &DotDirConfig{}

func (c *DotDirConfig) FieldName() string { return c.Name }
func (p *DotDirConfig) Init(ctx context.Context) error {
	if p.FS != nil {
		return nil
	}
	newfs := os.DirFS(p.Path)
	if _, err := newfs.(interface {
		Stat(string) (fs.FileInfo, error)
	}).Stat("."); err != nil {
		return fmt.Errorf("failed to stat fs current directory '%s': %w", p.Path, err)
	}
	p.FS = newfs
	return nil
}
func (p *DotDirConfig) Value(r Request) (any, error) {
	return Dir{dot: &dotFS{p.FS, GetLogger(r.R.Context()), make(map[fs.File]struct{})}, path: "."}, nil
}
func (p *DotDirConfig) Cleanup(a any, err error) error {
	v := a.(Dir).dot
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
