package xtemplate

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"time"

	"log/slog"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/infogulch/watch"
	"go.uber.org/zap/exp/zapslog"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

func init() {
	caddy.RegisterModule(XTemplateModule{})
}

// CaddyModule returns the Caddy module information.
func (XTemplateModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.xtemplate",
		New: func() caddy.Module { return new(XTemplateModule) },
	}
}

type XTemplateModule struct {
	// The root path from which to load template files within the selected
	// filesystem (the native filesystem by default). Default is the current
	// working directory.
	TemplateRoot string `json:"template_root,omitempty"`

	// The root path to reference files from within template funcs. The default,
	// empty string means the local filesystem funcs in templates are disabled.
	ContextRoot string `json:"context_root,omitempty"`

	// The template action delimiters. If set, must be precisely two elements:
	// the opening and closing delimiters. Default: `["{{", "}}"]`
	Delimiters []string `json:"delimiters,omitempty"`

	// The database driver and connection string. If set, must be precicely two
	// elements: the driver name and the connection string.
	Database struct {
		Driver  string `json:"driver,omitempty"`
		Connstr string `json:"connstr,omitempty"`
	} `json:"database,omitempty"`

	Config map[string]string `json:"config,omitempty"`

	FuncsModules []string `json:"funcs_modules,omitempty"`

	template *Templates
	halt     chan<- struct{}
}

// Validate ensures t has a valid configuration. Implements caddy.Validator.
func (m *XTemplateModule) Validate() error {
	if len(m.Delimiters) != 0 && len(m.Delimiters) != 2 {
		return fmt.Errorf("delimiters must consist of exactly two elements: opening and closing")
	}
	if m.Database.Driver != "" && slices.Index(sql.Drivers(), m.Database.Driver) == -1 {
		return fmt.Errorf("database driver '%s' does not exist", m.Database.Driver)
	}
	for _, m := range m.FuncsModules {
		mi, err := caddy.GetModule("xtemplate.funcs." + m)
		if err != nil {
			return fmt.Errorf("failed to find module 'xtemplate.funcs.%s': %v", m, err)
		}
		if _, ok := mi.New().(FuncsProvider); !ok {
			return fmt.Errorf("module 'xtemplate.funcs.%s' does not implement FuncsProvider", m)
		}
	}
	return nil
}

// Provision provisions t. Implements caddy.Provisioner.
func (m *XTemplateModule) Provision(ctx caddy.Context) error {
	log := slog.New(zapslog.NewHandler(ctx.Logger().Core(), nil))

	t := &Templates{
		Config: maps.Clone(m.Config),
		Log:    log,
	}

	var watchPaths []string

	// Context FS
	if m.ContextRoot != "" {
		cfs := os.DirFS(m.ContextRoot)
		if st, err := fs.Stat(cfs, "."); err != nil || !st.IsDir() {
			return fmt.Errorf("context file path does not exist in filesystem: %v", err)
		}
		t.ContextFS = cfs
		watchPaths = append(watchPaths, m.ContextRoot)
	}

	// Template FS
	{
		root := "."
		if len(m.TemplateRoot) > 0 {
			root = m.TemplateRoot
		}
		tfs := os.DirFS(root)
		if st, err := fs.Stat(tfs, "."); err != nil || !st.IsDir() {
			return fmt.Errorf("root file path does not exist in filesystem: %v", err)
		}
		t.TemplateFS = tfs
		watchPaths = append(watchPaths, root)
	}

	// ExtraFuncs
	{
		for _, m := range m.FuncsModules {
			mi, _ := caddy.GetModule("xtemplate.funcs." + m)
			fm := mi.New().(FuncsProvider).Funcs()
			log.Debug("got funcs from module", "module", "xtemplate.funcs."+m, "funcmap", fm)
			t.ExtraFuncs = append(t.ExtraFuncs, fm)
		}
	}

	if m.Database.Driver != "" {
		db, err := sql.Open(m.Database.Driver, m.Database.Connstr)
		if err != nil {
			return err
		}
		t.DB = db
	}

	if len(m.Delimiters) != 0 {
		t.Delims.L = m.Delimiters[0]
		t.Delims.R = m.Delimiters[1]
	} else {
		t.Delims.L = "{{"
		t.Delims.R = "}}"
	}

	{
		err := t.Reload()
		if err != nil {
			return err
		}
	}

	m.template = t

	{
		changed, halt, err := watch.WatchDirs(watchPaths, 200*time.Millisecond, log)
		if err != nil {
			return err
		}
		m.halt = halt
		go func() {
			for {
				select {
				case _, ok := <-changed:
					if !ok {
						return
					}
					err := t.Reload()
					if err != nil {
						log.Info("failed to reload xtemplate: %w", err)
					} else {
						log.Info("reloaded templates after file changed")
					}
				}
			}
		}()
	}
	return nil
}

// Cleanup discards resources held by t. Implements caddy.CleanerUpper.
func (m *XTemplateModule) Cleanup() error {
	if m.halt != nil {
		m.halt <- struct{}{}
		close(m.halt)
		m.halt = nil
	}
	if m.template != nil {
		var dberr error
		if m.template.DB != nil {
			dberr = m.template.DB.Close()
			m.template.DB = nil
		}
		m.template = nil
		return errors.Join(dberr)
	}
	return nil
}

func (m *XTemplateModule) ServeHTTP(w http.ResponseWriter, r *http.Request, _ caddyhttp.Handler) error {
	m.template.ServeHTTP(w, r)
	return nil
}

// Interface guards
var (
	_ caddy.Provisioner  = (*XTemplateModule)(nil)
	_ caddy.Validator    = (*XTemplateModule)(nil)
	_ caddy.CleanerUpper = (*XTemplateModule)(nil)

	_ caddyhttp.MiddlewareHandler = (*XTemplateModule)(nil)
)

func init() {
	httpcaddyfile.RegisterHandlerDirective("xtemplate", parseCaddyfile)
}

// parseCaddyfile sets up the handler from Caddyfile tokens. Syntax:
//
//	xtemplate [<matcher>] {
//	    database <driver> <connstr>
//	    delimiters <open_delim> <close_delim>
//	    root <path>
//	}
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	t := new(XTemplateModule)
	t.Config = make(map[string]string)
	for h.Next() {
		for h.NextBlock(0) {
			switch h.Val() {
			case "database":
				for nesting := h.Nesting(); h.NextBlock(nesting); {
					switch h.Val() {
					case "driver":
						if !h.Args(&t.Database.Driver) {
							return nil, h.ArgErr()
						}
					case "connstr":
						if !h.Args(&t.Database.Connstr) {
							return nil, h.ArgErr()
						}
					}
				}
			case "delimiters":
				t.Delimiters = h.RemainingArgs()
				if len(t.Delimiters) != 2 {
					return nil, h.ArgErr()
				}
			case "template_root":
				if !h.Args(&t.TemplateRoot) {
					return nil, h.ArgErr()
				}
			case "context_root":
				if !h.Args(&t.ContextRoot) {
					return nil, h.ArgErr()
				}
			case "config":
				for nesting := h.Nesting(); h.NextBlock(nesting); {
					var key, val string
					key = h.Val()
					if _, ok := t.Config[key]; ok {
						return nil, h.Errf("config key '%s' repeated", key)
					}
					if !h.Args(&val) {
						return nil, h.ArgErr()
					}
					t.Config[key] = val
				}
			case "funcs_modules":
				t.FuncsModules = h.RemainingArgs()
			default:
				return nil, h.Errf("unknown config option")
			}
		}
	}
	return t, nil
}

type FuncsProvider interface {
	Funcs() template.FuncMap
}
