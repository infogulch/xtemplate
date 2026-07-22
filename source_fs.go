package xtemplate

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/afero"
)

// FsSource serves templates from an in-process afero.Fs.
// Not JSON-configurable (no type registration); use WithTemplateFS.
type FsSource struct {
	FS afero.Fs `json:"-" arg:"-"`
}

// Start returns FS. FS must be non-nil.
func (s *FsSource) Start(ctx context.Context, log *slog.Logger) (afero.Fs, <-chan []Option, error) {
	if s.FS == nil {
		return nil, nil, fmt.Errorf("xtemplate: FsSource.FS is nil")
	}
	return s.FS, nil, nil
}
