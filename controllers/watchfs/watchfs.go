// Package watchfs is a ServerController that serves templates from a local
// directory and reloads when watched paths change. JSON type: "watchfs".
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
	xtemplate.RegisterController("watchfs", func() xtemplate.ServerController { return &Controller{} })
}

// Controller watches Path (and optional WatchDirs) and reloads on change.
type Controller struct {
	// Path is the templates directory. Default "templates".
	Path string `json:"path,omitempty" arg:"-t,--template-dir,--templates-dir" default:"templates"`

	// WatchDirs lists extra directories to watch (in addition to Path).
	WatchDirs []string `json:"watch_dirs,omitempty" arg:"--watch,separate"`

	// Debounce is the fs event debounce interval. Default 200ms.
	Debounce xtemplate.Duration `json:"debounce,omitempty" arg:"--debounce"`

	path     string
	debounce time.Duration
	dirs     []string
	log      *slog.Logger
}

// Init returns the directory FS as sticky.
func (s *Controller) Init(_ context.Context, log *slog.Logger) ([]xtemplate.Option, error) {
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
	s.path = path
	s.debounce = debounce
	s.log = log
	s.dirs = append(append([]string{}, s.WatchDirs...), path)

	fs := afero.NewBasePathFs(afero.NewOsFs(), path)
	return []xtemplate.Option{xtemplate.WithTemplateFS(fs)}, nil
}

// Start watches for changes and reloads from sticky. Stops when server.Context() is cancelled.
func (s *Controller) Start(server *xtemplate.Server) error {
	log := s.log
	if log == nil {
		log = slog.Default()
	}
	ctx := server.Context()

	halt, err := watch.Watch(s.dirs, s.debounce, log.WithGroup("fswatch"), func() bool {
		if ctx.Err() != nil {
			return false
		}
		if err := server.Reload(); err != nil {
			log.Error("reload failed", slog.Any("error", err))
		}
		return true
	})
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		close(halt)
	}()
	return nil
}
