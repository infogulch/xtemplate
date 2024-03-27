// xtemplate extends Go's html/template to be capable enough to define an entire
// server-side application with a directory of Go templates.
package xtemplate

import (
	"context"
	"html/template"
	"io/fs"
	"log/slog"
)

func New() (c *Config) {
	c = &Config{}
	c.Defaults()
	return
}

type Config struct {
	// The path to the templates directory. Default `templates`.
	TemplatesDir string `json:"templates_dir,omitempty" arg:"-t,--template-dir" default:"templates"`

	// The FS to load templates from. Overrides Path if not nil.
	TemplatesFS fs.FS `json:"-" arg:"-"`

	// File extension to search for to find template files. Default `.html`.
	TemplateExtension string `json:"template_extension,omitempty" arg:"--template-ext" default:".html"`

	// Left template action delimiter. Default `{{`.
	LDelim string `json:"left,omitempty" arg:"--ldelim" default:"{{"`

	// Right template action delimiter. Default `}}`.
	RDelim string `json:"right,omitempty" arg:"--rdelim" default:"}}"`

	// Whether html templates are minified at load time. Default `true`.
	Minify bool `json:"minify,omitempty" arg:"-m,--minify" default:"true"`

	// A list of additional custom fields to add to the template dot value
	// `{{.}}`.
	Dot []DotConfig `json:"dot" arg:"-d,--dot,separate"`

	// Additional functions to add to the template execution context.
	FuncMaps []template.FuncMap `json:"-" arg:"-"`

	// The instance context that is threaded through dot providers and can
	// cancel the server. Defaults to `context.Background()`.
	Ctx context.Context `json:"-" arg:"-"`

	// The default logger. Defaults to `slog.Default()`.
	Logger *slog.Logger `json:"-" arg:"-"`
}

// FillDefaults sets default values for unset fields
func (config *Config) Defaults() *Config {
	if config.TemplatesDir == "" {
		config.TemplatesDir = "templates"
	}

	if config.TemplateExtension == "" {
		config.TemplateExtension = ".html"
	}

	if config.LDelim == "" {
		config.LDelim = "{{"
	}

	if config.RDelim == "" {
		config.RDelim = "}}"
	}

	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	if config.Ctx == nil {
		config.Ctx = context.Background()
	}

	return config
}

type ConfigOverride func(*Config)

func WithTemplateFS(fs fs.FS) ConfigOverride {
	return func(c *Config) {
		c.TemplatesFS = fs
	}
}

func WithLogger(logger *slog.Logger) ConfigOverride {
	return func(c *Config) {
		c.Logger = logger
	}
}

func WithFuncMaps(fm ...template.FuncMap) ConfigOverride {
	return func(c *Config) {
		c.FuncMaps = append(c.FuncMaps, fm...)
	}
}

func WithProvider(name string, p DotProvider) ConfigOverride {
	return func(c *Config) {
		for _, d := range c.Dot {
			if d.Name == name {
				if d.DotProvider != p {
					c.Logger.Warn("tried to assign different providers the same name", slog.String("name", d.Name), slog.Any("old", d.DotProvider), slog.Any("new", p))
				}
				return
			}
		}
		c.Dot = append(c.Dot, DotConfig{Name: name, DotProvider: p})
	}
}
