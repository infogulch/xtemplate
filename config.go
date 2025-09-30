// Package xtemplate extends Go's html/template to be capable enough to define an entire
// server-side application with a directory of Go templates.
package xtemplate

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"

	"github.com/infogulch/xtemplate/backends"
	"github.com/spf13/afero"
)

func New() (c *Config) {
	c = &Config{}
	c.Defaults()
	return
}

type Config struct {
	// The path to the templates directory within the filesystem. Default `templates`.
	TemplatesDir string `json:"templates_dir,omitempty" arg:"-t,--template-dir" default:"templates"`

	// The FS to load templates from. Default: a FS made from the current working directory.
	TemplatesFS afero.Fs `json:"-" arg:"-"`

	// Backend provides the template storage backend (filesystem, NATS Object Store, etc.)
	// If nil, defaults to filesystem backend
	Backend backends.Backend `json:"-" arg:"-"`

	// File extension to search for to find template files. Default `.html`.
	TemplateExtension string `json:"template_extension,omitempty" arg:"--template-ext" default:".html"`

	// Whether html templates are minified at load time. Default `true`.
	Minify bool `json:"minify,omitempty" arg:"-m,--minify" default:"true"`

	Databases       []DotDBConfig     `json:"databases" arg:"-"`
	Flags           []DotFlagsConfig  `json:"flags" arg:"-"`
	Directories     []DotDirConfig    `json:"directories" arg:"-"`
	Nats            []*DotNatsConfig  `json:"nats" arg:"-"`
	CustomProviders []DotConfig       `json:"-" arg:"-"`

	// Left template action delimiter. Default `{{`.
	LDelim string `json:"left,omitempty" arg:"--ldelim" default:"{{"`

	// Right template action delimiter. Default `}}`.
	RDelim string `json:"right,omitempty" arg:"--rdelim" default:"}}"`

	// Additional functions to add to the template execution context.
	FuncMaps []template.FuncMap `json:"-" arg:"-"`

	// The instance context that is threaded through dot providers and can
	// cancel the server. Defaults to `context.Background()`.
	Ctx context.Context `json:"-" arg:"-"`

	// The default logger. Defaults to `slog.Default()`.
	Logger *slog.Logger `json:"-" arg:"-"`
}

// Defaults sets default values for unset fields
func (c *Config) Defaults() *Config {
	if c.TemplatesDir == "" {
		c.TemplatesDir = "templates"
	}

	if c.TemplateExtension == "" {
		c.TemplateExtension = ".html"
	}

	if c.LDelim == "" {
		c.LDelim = "{{"
	}

	if c.RDelim == "" {
		c.RDelim = "}}"
	}

	if c.Logger == nil {
		c.Logger = slog.Default()
	}

	if c.Ctx == nil {
		c.Ctx = context.Background()
	}

	return c
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

func WithTemplateFS(fs afero.Fs) Option {
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

func WithProvider(p DotConfig) Option {
	return func(c *Config) error {
		c.CustomProviders = append(c.CustomProviders, p)
		return nil
	}
}
