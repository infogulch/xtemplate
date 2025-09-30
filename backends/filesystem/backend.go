package filesystem

import (
	"github.com/infogulch/xtemplate/backends"
	"github.com/spf13/afero"
)

// Backend implements the backends.Backend interface for filesystem storage
type Backend struct {
	fs      afero.Fs
	watcher backends.Watcher
}

// New creates a new filesystem backend with the given base path and optional watch directories
func New(basePath string, watchDirs []string) *Backend {
	fs := afero.NewBasePathFs(afero.NewOsFs(), basePath)

	var watcher backends.Watcher
	if len(watchDirs) > 0 {
		watcher = NewWatcher(watchDirs)
	}

	return &Backend{
		fs:      fs,
		watcher: watcher,
	}
}

// NewWithFS creates a new filesystem backend with a custom afero.Fs and optional watch directories
func NewWithFS(fs afero.Fs, watchDirs []string) *Backend {
	var watcher backends.Watcher
	if len(watchDirs) > 0 {
		watcher = NewWatcher(watchDirs)
	}

	return &Backend{
		fs:      fs,
		watcher: watcher,
	}
}

// FS returns the afero.Fs implementation for this backend
func (b *Backend) FS() afero.Fs {
	return b.fs
}

// Watcher returns the Watcher implementation for this backend
func (b *Backend) Watcher() backends.Watcher {
	return b.watcher
}

// Name returns the name of this backend
func (b *Backend) Name() string {
	return "filesystem"
}

