// xtemplate extends Go's html/template to be capable enough to define an entire
// server-side application with a directory of Go templates.
package xtemplate

import (
	"database/sql"
	"html/template"
	"io/fs"
	"log/slog"
)

func New() (c *Config) {
	c = &Config{}
	c.Template.Path = "templates"
	c.UserConfig = make(map[string]string)
	return
}

type Config struct {
	// Control where and how templates are loaded.
	Template struct {
		// The FS to load templates from. Overrides Path if not nil.
		FS fs.FS `json:"-"`

		// The path to the templates directory.
		Path string `json:"path,omitempty"`

		// File extension to search for to find template files. Default `.html`.
		TemplateExtension string `json:"template_extension,omitempty"`

		// The template action delimiters, default "{{" and "}}".
		Delimiters struct {
			Left  string `json:"left,omitempty"`
			Right string `json:"right,omitempty"`
		} `json:"delimiters,omitempty"`
	} `json:"template,omitempty"`

	// Control where the templates may have dynamic access the filesystem.
	Context struct {
		// The FS to give dynamic access to templates. Overrides Path if not nil.
		FS fs.FS `json:"-"`

		// Path to a directory to give dynamic access to templates.
		Path string `json:"path,omitempty"`
	} `json:"context,omitempty"`

	// The database driver and connection string. If set, must be precicely two
	// elements: the driver name and the connection string.
	Database struct {
		DB      *sql.DB `json:"-"`
		Driver  string  `json:"driver,omitempty"`
		Connstr string  `json:"connstr,omitempty"`
	} `json:"database,omitempty"`

	// User configration, accessible in the template execution context as `.Config`.
	UserConfig UserConfig `json:"config,omitempty"`

	// Additional functions to add to the template execution context.
	FuncMaps []template.FuncMap `json:"-"`

	Logger   *slog.Logger `json:"-"`
	LogLevel int          `json:"log_level,omitempty"`
}

type UserConfig map[string]string

func (c *Config) WithTemplateFS(fs fs.FS) {
	c.Template.FS = fs
}

type override func(*Config)

func WithContextFS(fs fs.FS) override {
	return func(c *Config) {
		c.Context.FS = fs
	}
}

func WithDB(db *sql.DB) override {
	return func(c *Config) {
		c.Database.DB = db
	}
}

func WithLogger(logger *slog.Logger) override {
	return func(c *Config) {
		c.Logger = logger
	}
}

func WithFuncMaps(fm ...template.FuncMap) override {
	return func(c *Config) {
		c.FuncMaps = append(c.FuncMaps, fm...)
	}
}
