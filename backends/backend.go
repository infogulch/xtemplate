package backends

import (
	"log/slog"
	"time"

	"github.com/spf13/afero"
)

// Backend represents a template storage backend that provides both
// filesystem access and optional watch capabilities.
type Backend interface {
	// FS returns the afero.Fs implementation for this backend
	FS() afero.Fs

	// Watcher returns the Watcher implementation for this backend, or nil if watching is not supported
	Watcher() Watcher

	// Name returns a human-readable name for this backend (e.g., "filesystem", "nats-objstore")
	Name() string
}

// Watcher monitors for changes and triggers reload callbacks
type Watcher interface {
	// Start begins watching and returns a channel to halt watching
	Start(debounce time.Duration, log *slog.Logger, onchange func() bool) (halt chan<- struct{}, err error)
}

