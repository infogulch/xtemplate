package providers

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

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
	return &DotFS{p.FS, xtemplate.GetLogger(r.R.Context()), r.W, r.R, make(map[fs.File]struct{})}, nil
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
