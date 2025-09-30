package filesystem

import (
	"log/slog"
	"time"

	"github.com/infogulch/watch"
)

// Watcher watches filesystem directories for changes
type Watcher struct {
	dirs []string
}

// NewWatcher creates a new filesystem watcher for the given directories
func NewWatcher(dirs []string) *Watcher {
	return &Watcher{dirs: dirs}
}

// Start begins watching the filesystem directories
func (w *Watcher) Start(debounce time.Duration, log *slog.Logger, onchange func() bool) (halt chan<- struct{}, err error) {
	return watch.Watch(w.dirs, debounce, log, onchange)
}

