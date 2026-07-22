package xtemplate

import (
	"context"
	"log/slog"

	"github.com/spf13/afero"
)

func init() {
	RegisterController("os", func() ServerController { return &OsFsController{} })
}

// OsFsController serves templates from a local directory (JSON type "os").
type OsFsController struct {
	// Path is the templates directory. Default "templates".
	Path string `json:"path,omitempty" arg:"-t,--template-dir,--templates-dir" default:"templates"`
}

var _ ServerController = (*OsFsController)(nil)

// Init returns a sticky BasePathFs rooted at Path (default "templates").
func (s *OsFsController) Init(_ context.Context, _ *slog.Logger) ([]Option, error) {
	path := s.Path
	if path == "" {
		path = "templates"
	}
	fs := afero.NewBasePathFs(afero.NewOsFs(), path)
	return []Option{WithTemplateFS(fs)}, nil
}

func (s *OsFsController) Start(_ *Server) error { return nil }
