// xtemplate extends Go's html/template to be capable enough to define an entire
// server-side application with a directory of Go templates.
package xtemplate

import (
	"context"
	"fmt"
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

func (c *Config) Options(options ...Option) (*Config, error) {
	for _, o := range options {
		if err := o(c); err != nil {
			return nil, fmt.Errorf("failed to apply xtemplate config option: %w", err)
		}
	}
	return c, nil
}

type Option func(*Config) error

func WithTemplateFS(fs fs.FS) Option {
	return func(c *Config) error {
		if fs == nil {
			return fmt.Errorf("nil fs")
		}
		c.TemplatesFS = fs
		return nil
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(c *Config) error {
		if logger == nil {
			return fmt.Errorf("nil logger")
		}
		c.Logger = logger
		return nil
	}
}

func WithFuncMaps(fm ...template.FuncMap) Option {
	return func(c *Config) error {
		c.FuncMaps = append(c.FuncMaps, fm...)
		return nil
	}
}

func WithProvider(name string, p DotProvider) Option {
	return func(c *Config) error {
		for _, d := range c.Dot {
			if d.Name == name {
				if d.DotProvider != p {
					return fmt.Errorf("tried to assign different providers the same name. name: %s; old: %v; new: %v", d.Name, d.DotProvider, p)
				}
				return nil
			}
		}
		c.Dot = append(c.Dot, DotConfig{Name: name, DotProvider: p})
		return nil
	}
}
