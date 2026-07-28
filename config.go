// xtemplate extends Go's html/template to be capable enough to define an entire
// server-side application with a directory of Go templates.
package xtemplate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"maps"
	"net/http"
	"slices"

	"github.com/spf13/afero"
)

func New() (c *Config) {
	c = &Config{}
	c.SetDefaults()
	return
}

type Config struct {
	// Controller is the optional ServerController (JSON key "controller").
	// Resolved from ControllerRaw by [Config.MaterializeController] (LoadConfig /
	// Server.construct). Instance requires TemplateFS instead and rejects
	// Controller / ControllerRaw.
	Controller ServerController `json:"-" arg:"-"`

	// ControllerRaw is the JSON "controller" object until MaterializeController.
	ControllerRaw json.RawMessage `json:"controller,omitempty" arg:"-"`

	// TemplateFS is the private build-root FS (not JSON). Sticky after controller
	// Init or WithTemplateFS/Dir; also the per-Reload root when set via options.
	TemplateFS afero.Fs `json:"-" arg:"-"`

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

// SetDefaults fills unset fields. Does not choose a Controller or clone slices.
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

// cloneSlices replaces every append-owned slice with a copy so later Options
// or builds cannot mutate the caller's (or sticky Server's) backing arrays.
// Maps inside FuncMaps are cloned; Provider values and Handler refs are not deep-copied.
func (c *Config) cloneSlices() {
	c.ControllerRaw = slices.Clone(c.ControllerRaw)
	if c.ProvidersRaw != nil {
		out := make([]json.RawMessage, len(c.ProvidersRaw))
		for i, raw := range c.ProvidersRaw {
			out[i] = slices.Clone(raw)
		}
		c.ProvidersRaw = out
	}
	c.Providers = slices.Clone(c.Providers)
	c.Precompress = slices.Clone(c.Precompress)
	if c.FuncMaps != nil {
		out := make([]template.FuncMap, len(c.FuncMaps))
		for i, fm := range c.FuncMaps {
			out[i] = maps.Clone(fm)
		}
		c.FuncMaps = out
	}
	c.Handlers = slices.Clone(c.Handlers)
	c.onClose = slices.Clone(c.onClose)
	c.CrossOrigin.TrustedOrigins = slices.Clone(c.CrossOrigin.TrustedOrigins)
	c.CrossOrigin.InsecureBypassPatterns = slices.Clone(c.CrossOrigin.InsecureBypassPatterns)
}

// bannedTemplateKeys hard-reject renamed top-level JSON keys with migrate text.
// REMOVE BEFORE 1.0.
var bannedTemplateKeys = map[string]string{
	"templates_dir":       `templates_dir is no longer supported; use "controller": {"type":"os","path":"…"} (or watchfs/git as appropriate)`,
	"templates_path":      `templates_path is no longer supported; use "controller": {"type":"os","path":"…"} (or watchfs/git as appropriate)`,
	"watch_dirs":          `watch_dirs is no longer supported; use "controller": {"type":"watchfs","path":"…","watch_dirs":[…]}`,
	"watch_template_path": `watch_template_path is no longer supported; use controller os or watchfs to control reload behavior`,
	"git_repo":            `git_repo is no longer supported; use "controller": {"type":"git","repo":"…"}`,
	"git_ref":             `git_ref is no longer supported; use "controller": {"type":"git","ref":"…"}`,
	"git_interval":        `git_interval is no longer supported; use "controller": {"type":"git","interval":"…"}`,
}

// CheckLegacyTemplateKeys returns a migrate error if banned top-level keys are present.
func CheckLegacyTemplateKeys(data []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	for key, msg := range bannedTemplateKeys {
		if _, ok := m[key]; ok {
			return fmt.Errorf("xtemplate: %s", msg)
		}
	}
	return nil
}

// UnmarshalJSON applies the ban-list then unmarshals into Config.
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

type Option func(*Config) error

// Options applies the given options to the Config, returning the updated Config
// or the first error.
func (c *Config) Options(options ...Option) (*Config, error) {
	var errs error
	for _, o := range options {
		if err := o(c); err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to apply xtemplate config option: %w", err))
		}
	}
	return c, errs
}

// WithController sets the ServerController. Rejected when building an Instance / Reload.
func WithController(s ServerController) Option {
	return func(c *Config) error {
		if s == nil {
			return fmt.Errorf("nil controller")
		}
		c.Controller = s
		return nil
	}
}

// WithTemplateFS sets the private build-root FS for the next Instance.
func WithTemplateFS(fs afero.Fs) Option {
	return func(c *Config) error {
		if fs == nil {
			return fmt.Errorf("nil fs")
		}
		c.TemplateFS = fs
		return nil
	}
}

// WithTemplateDir sets the private build-root FS to an OS directory.
func WithTemplateDir(dir string) Option {
	return func(c *Config) error {
		if dir == "" {
			return fmt.Errorf("empty template dir")
		}
		c.TemplateFS = afero.NewBasePathFs(afero.NewOsFs(), dir)
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
// is [Instance.Close]d (reload retire or stop). Multiple callbacks append; they
// run after provider [Closer]s, reverse registration order. Nil fns ignored.
//
// Per instance, not once per Server: on the sticky base, fn runs for every
// retired instance. Use sync.Once for process-wide cleanup, or pass WithOnClose
// only on the Reload that owns a per-build resource (e.g. a temp clone dir).
func WithOnClose(fn func() error) Option {
	return func(c *Config) error {
		if fn == nil {
			return nil
		}
		c.onClose = append(c.onClose, fn)
		return nil
	}
}

func WithContext(ctx context.Context) Option {
	return func(c *Config) error {
		c.Ctx = ctx
		return nil
	}
}

func WithMinify(minify bool) Option {
	return func(c *Config) error {
		c.Minify = &minify
		return nil
	}
}
