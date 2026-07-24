// xtemplate extends Go's html/template to be capable enough to define an entire
// server-side application with a directory of Go templates.
package xtemplate

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/spf13/afero"
)

func New() (c *Config) {
	c = &Config{}
	c.SetDefaults()
	return
}

type Config struct {
	// Source is the template lifecycle provider for this Server/Instance.
	// Set via WithSource, WithTemplateFS/Dir (Server invents a Source if needed),
	// or JSON SourceRaw (resolved by LoadConfig / Server / Instance).
	Source TemplateSource `json:"-" arg:"-"`

	// SourceRaw is the JSON "source" object. Resolved into Source by LoadConfig
	// or Server/Instance; cleared after materialization.
	SourceRaw json.RawMessage `json:"source,omitempty" arg:"-"`

	// templatesFS is the private build root for the next Instance. Not JSON;
	// not public API. Filled by Source.Start or WithTemplateFS/Dir.
	templatesFS afero.Fs

	// reloadResult is set by WithReloadResult and invoked by Server.Reload with
	// the outcome of applying that option set (success or failure).
	reloadResult func(error)

	// File extension to search for to find template files. Default `.html`.
	TemplateExtension string `json:"template_extension,omitempty" arg:"--template-ext" default:".html"`

	// Whether html templates are minified at load time. Default `true`.
	//
	// This is a *bool to distinguish unset (nil) from set false.
	Minify *bool `json:"minify,omitempty" arg:"-m,--minify" default:"true"`

	CrossOrigin CrossOriginConfig `json:"crossorigin" arg:"-"`

	ProvidersRaw []json.RawMessage `json:"providers,omitempty" arg:"-"`
	Providers    []Provider        `json:"-" arg:"-"`

	// Encodings to pre-compress static files into at load time. Supported values:
	// "gzip", "zstd", "br". Default empty (no pre-compression).
	Precompress []string `json:"precompress,omitempty" arg:"--precompress,separate"`

	// Left template action delimiter. Default `{{`.
	LDelim string `json:"left,omitempty" arg:"--ldelim" default:"{{"`

	// Right template action delimiter. Default `}}`.
	RDelim string `json:"right,omitempty" arg:"--rdelim" default:"}}"`

	// Additional functions to add to the template execution context.
	FuncMaps []template.FuncMap `json:"-" arg:"-"`

	// Peer HTTP handlers registered on the instance ServeMux next to template
	// and static routes. Use this to embed existing handlers (APIs, webhooks,
	// health checks) under the same ServeMux as the template app. Errors if the
	// pattern conflicts with another route registered by the template root.
	//
	// Each entry is a sibling route on the mux (net/http.ServeMux pattern).
	// Re-registered when a new instance is built, so they survive reloads.
	Handlers []HandlerRoute `json:"-" arg:"-"`

	// The instance context that is threaded through dot providers and can
	// cancel the server. Defaults to `context.Background()`.
	Ctx context.Context `json:"-" arg:"-"`

	// The default logger. Defaults to `slog.Default()`.
	Logger *slog.Logger `json:"-" arg:"-"`

	// onClose callbacks for the next Instance built from this config.
	// See [WithOnClose]. Not JSON. Each Instance takes a reslice at build time.
	onClose []func() error
}

// HandlerRoute pairs a ServeMux pattern with an http.Handler.
type HandlerRoute struct {
	// Pattern uses net/http.ServeMux syntax (e.g. "POST /foo/{bar}").
	Pattern string
	Handler http.Handler
}

type CrossOriginConfig struct {
	Disabled               bool     `json:"disabled" arg:"--disable-cors" default:"false"`
	TrustedOrigins         []string `json:"trusted_origins" arg:"--trusted-origin,separate"`
	InsecureBypassPatterns []string `json:"insecure_bypass_patterns" arg:"--insecure-bypass-pattern,separate"`
}

// FillDefaults sets default values for unset fields.
// Does not install a default Source (Server uses os path "templates" when unset).
func (config *Config) SetDefaults() *Config {
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

	if config.Minify == nil {
		defaultMinify := true
		config.Minify = &defaultMinify
	}

	return config
}

// UnmarshalJSON applies the legacy-key ban-list then unmarshals into Config.
// REMOVE BEFORE 1.0: temporary hard-rejects for renamed top-level keys.
func (c *Config) UnmarshalJSON(data []byte) error {
	if err := CheckLegacyTemplateKeys(data); err != nil {
		return err
	}
	type alias Config
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*c = Config(a)
	return nil
}

// resolveSourceRaw materializes Source from SourceRaw when Source is unset.
// Clears SourceRaw after a successful resolve (or when Source was already set).
func (c *Config) resolveSourceRaw() error {
	if len(c.SourceRaw) == 0 {
		return nil
	}
	if c.Source != nil {
		c.SourceRaw = nil
		return nil
	}
	s, err := ResolveSource(c.SourceRaw)
	if err != nil {
		return err
	}
	c.Source = s
	c.SourceRaw = nil
	return nil
}

// ensureSource sets a default Source when neither Source nor templatesFS is set
// (os path "templates"), or wraps an existing templatesFS in FsSource.
// Call after resolveSourceRaw. Used by Server; Instance does not invent defaults.
func (c *Config) ensureSource() {
	if c.Source != nil {
		return
	}
	if c.templatesFS == nil {
		c.Source = &OsFsSource{"templates"}
	} else {
		c.Source = &FsSource{FS: c.templatesFS}
	}
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

// WithSource sets the template Source and clears templatesFS so Start fills it.
// Illegal during Reload.
func WithSource(s TemplateSource) Option {
	return func(c *Config) error {
		if s == nil {
			return fmt.Errorf("nil source")
		}
		c.Source = s
		c.templatesFS = nil
		return nil
	}
}

// WithReloadResult registers fn to be called once with the result of
// [Server.Reload] for this option set (nil on success). Used by sources that
// need success/failure feedback (e.g. git last-SHA advancement). fn must not
// block. No-op for standalone [Config.Instance] builds.
func WithReloadResult(fn func(error)) Option {
	return func(c *Config) error {
		if fn == nil {
			return nil
		}
		prev := c.reloadResult
		c.reloadResult = func(err error) {
			if prev != nil {
				prev(err)
			}
			fn(err)
		}
		return nil
	}
}

// WithTemplateFS sets the private build-root FS for the next Instance.
func WithTemplateFS(fs afero.Fs) Option {
	return func(c *Config) error {
		if fs == nil {
			return fmt.Errorf("nil fs")
		}
		c.templatesFS = fs
		return nil
	}
}

// WithTemplateDir sets the private build-root FS to an OS directory.
func WithTemplateDir(dir string) Option {
	return func(c *Config) error {
		if dir == "" {
			return fmt.Errorf("empty template dir")
		}
		c.templatesFS = afero.NewBasePathFs(afero.NewOsFs(), dir)
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

// WithHandler mounts h at pattern on the instance ServeMux (net/http.ServeMux
// syntax, e.g. "POST /api/{id}" or "GET /healthz"). Appends to Config.Handlers.
//
// Intended for embedding foreign handlers (API, webhooks, probes) as peer
// routes beside templates and static files.
func WithHandler(pattern string, h http.Handler) Option {
	return func(c *Config) error {
		if h == nil {
			return fmt.Errorf("nil handler for pattern %q", pattern)
		}
		c.Handlers = append(c.Handlers, HandlerRoute{pattern, h})
		return nil
	}
}

func WithProvider(p Provider) Option {
	return func(c *Config) error {
		c.Providers = append(c.Providers, p)
		return nil
	}
}

// WithOnClose registers fn to run when the [Instance] built with this option
// is [Instance.Close]d (reload retire or [Server.Stop]/[Server.Shutdown]).
// Multiple WithOnClose options append; they run after provider [Closer]s, in
// reverse registration order. Nil fns are ignored.
//
// Callbacks are per instance, not once per Server: if set on the Server base
// config (e.g. Server(WithOnClose(fn))), fn runs once for every retired
// instance after each successful Reload and on final stop. That is intentional
// for hooks like metrics; for one-shot process cleanup wrap with sync.Once.
// For per-build resources (e.g. a git clone directory), pass WithOnClose only
// on the Reload options that install that build—not on the base config—so each
// clone is released with the instance that adopted it.
//
// Each Instance stores its own reslice of handlers so Options appends during
// one build cannot trample another instance's list or the base config slice.
func WithOnClose(fn func() error) Option {
	return func(c *Config) error {
		if fn == nil {
			return nil
		}
		c.onClose = append(c.onClose, fn)
		return nil
	}
}
