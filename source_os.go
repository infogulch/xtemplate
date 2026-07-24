package xtemplate

import (
	"context"
	"log/slog"

	"github.com/spf13/afero"
)

func init() {
	RegisterSource("os", func() TemplateSource { return &OsFsSource{} })
}

// OsFsSource serves templates from a directory on the local OS filesystem.
// JSON type: "os". Does not emit reloads.
type OsFsSource struct {
	// Path is the templates directory. Default "templates".
	Path string `json:"path,omitempty" arg:"-t,--template-dir,--templates-dir" default:"templates"`
}

// Start returns a BasePathFs rooted at Path (default "templates").
func (s *OsFsSource) Start(ctx context.Context, log *slog.Logger) (afero.Fs, <-chan []Option, error) {
	path := s.Path
	if path == "" {
		path = "templates"
	}
	return afero.NewBasePathFs(afero.NewOsFs(), path), nil, nil
}
