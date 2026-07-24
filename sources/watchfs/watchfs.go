// Package watchfs is a TemplateSource that serves templates from a local
// directory and emits empty reload options when watched paths change.
package watchfs

import (
	"context"
	"log/slog"
	"time"

	"github.com/infogulch/watch"
	"github.com/infogulch/xtemplate"
	"github.com/spf13/afero"
)

func init() {
	xtemplate.RegisterSource("watchfs", func() xtemplate.TemplateSource { return &Source{} })
}

// Source watches Path (and optional WatchDirs) and reloads on change.
// JSON type: "watchfs".
type Source struct {
	// Path is the templates directory. Default "templates".
	Path string `json:"path,omitempty" arg:"-t,--template-dir,--templates-dir" default:"templates"`

	// WatchDirs lists extra directories to watch (in addition to Path).
	WatchDirs []string `json:"watch_dirs,omitempty" arg:"--watch,separate"`

	// Debounce is the fs event debounce interval. Default 200ms.
	Debounce xtemplate.Duration `json:"debounce,omitempty" arg:"--debounce"`
}

// Start returns the directory FS and a channel that emits empty option slices
// on filesystem changes. Stops when ctx is cancelled.
func (s *Source) Start(ctx context.Context, log *slog.Logger) (afero.Fs, <-chan []xtemplate.Option, error) {
	path := s.Path
	if path == "" {
		path = "templates"
	}
	debounce := s.Debounce.Duration()
	if debounce == 0 {
		debounce = 200 * time.Millisecond
	}
	if log == nil {
		log = slog.Default()
	}

	dirs := append([]string{}, s.WatchDirs...)
	dirs = append(dirs, path)

	ch := make(chan []xtemplate.Option)
	halt, err := watch.Watch(dirs, debounce, log.WithGroup("fswatch"), func() bool {
		select {
		case <-ctx.Done():
			return false
		case ch <- nil:
			return true
		}
	})
	if err != nil {
		return nil, nil, err
	}

	go func() {
		<-ctx.Done()
		close(halt)
		// Do not close ch: consumer exits on serverCtx; avoid send-on-closed races.
	}()

	initial := afero.NewBasePathFs(afero.NewOsFs(), path)
	return initial, ch, nil
}
