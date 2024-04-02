package wfs

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	hos "github.com/hack-pad/hackpadfs/os"
	"github.com/infogulch/xtemplate"
)

func init() {
	xtemplate.RegisterDot(&DotWFSProvider{})
}

// DotWFSProvider can configure an xtemplate dot field to provide file system
// access to templates. You can configure xtemplate to use it three ways:
//
// By setting a cli flag: “
type DotWFSProvider struct {
	fs.FS `json:"-"`
	Path  string `json:"path"`
}

var _ encoding.TextMarshaler = &DotWFSProvider{}

func (fs *DotWFSProvider) MarshalText() ([]byte, error) {
	if fs.Path == "" {
		return nil, fmt.Errorf("FSDir cannot be marhsaled")
	}
	return []byte(fs.Path), nil
}

var _ encoding.TextUnmarshaler = &DotWFSProvider{}

func (fs *DotWFSProvider) UnmarshalText(b []byte) error {
	dir := string(b)
	if dir == "" {
		return fmt.Errorf("fs dir cannot be empty string")
	}
	var err error
	fs.FS, err = hos.NewFS().Sub(dir)
	return err
}

var _ json.Marshaler = &DotWFSProvider{}

func (d *DotWFSProvider) MarshalJSON() ([]byte, error) {
	type T DotWFSProvider
	return json.Marshal((*T)(d))
}

var _ json.Unmarshaler = &DotWFSProvider{}

func (d *DotWFSProvider) UnmarshalJSON(b []byte) error {
	type T DotWFSProvider
	return json.Unmarshal(b, (*T)(d))
}

var _ xtemplate.CleanupDotProvider = &DotWFSProvider{}

func (DotWFSProvider) New() xtemplate.DotProvider { return &DotWFSProvider{} }
func (DotWFSProvider) Type() string               { return "wfs" }
func (p *DotWFSProvider) Value(r xtemplate.Request) (any, error) {
	if p.FS == nil {
		newfs, err := hos.NewFS().Sub(p.Path)
		if err != nil {
			return &DotWFS{}, fmt.Errorf("failed to create wfs: %w", err)
		}
		if _, err := newfs.(interface {
			Stat(string) (fs.FileInfo, error)
		}).Stat("."); err != nil {
			return &DotWFS{}, fmt.Errorf("failed to stat fs current directory '%s': %w", p.Path, err)
		}
		p.FS = newfs
	}
	return &DotWFS{p.FS, xtemplate.GetLogger(r.R.Context()), r.W, r.R, make(map[fs.File]struct{})}, nil
}
func (p *DotWFSProvider) Cleanup(a any, err error) error {
	v := a.(*DotWFS)
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
