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
	// The FS to load templates from. Overrides Path if not nil.
	FS fs.FS `json:"-" arg:"-"`

	// The path to the templates directory.
	TemplatesDir string `json:"templates_dir,omitempty" arg:"-t,--template-dir" default:"templates"`

	// File extension to search for to find template files. Default `.html`.
	TemplateExtension string `json:"template_extension,omitempty" arg:"--template-ext" default:".html"`

	// The template action delimiters, default "{{" and "}}".
	LDelim string `json:"left,omitempty" arg:"--ldelim" default:"{{"`
	RDelim string `json:"right,omitempty" arg:"--rdelim" default:"}}"`

	// Minify html templates at load time.
	Minify bool `json:"minify,omitempty" arg:"-m,--minify" default:"true"`

	Dot []DotConfig `json:"dot_config" arg:"-c,--dot-config,separate"`

	// Additional functions to add to the template execution context.
	FuncMaps []template.FuncMap `json:"-" arg:"-"`
	Ctx      context.Context    `json:"-" arg:"-"`

	Logger   *slog.Logger `json:"-" arg:"-"`
	LogLevel int          `json:"log_level,omitempty"`
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

	return config
}

type ConfigOverride func(*Config)

func WithTemplateFS(fs fs.FS) ConfigOverride {
	return func(c *Config) {
		c.FS = fs
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
		c.Dot = append(c.Dot, DotConfig{Name: name, DotProvider: p})
	}
}
