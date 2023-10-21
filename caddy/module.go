package xtemplate_caddy

import (
	"database/sql"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"time"

	"log/slog"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/infogulch/watch"
	"github.com/infogulch/xtemplate"
	"go.uber.org/zap/exp/zapslog"
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

	handler http.Handler
	db      *sql.DB
	halt    chan<- struct{}
}

// Validate ensures t has a valid configuration. Implements caddy.Validator.
func (m *XTemplateModule) Validate() error {
	if len(m.Delimiters) != 0 && len(m.Delimiters) != 2 {
		return fmt.Errorf("delimiters must consist of exactly two elements: opening and closing")
	}
	if m.Database.Driver != "" && slices.Index(sql.Drivers(), m.Database.Driver) == -1 {
		return fmt.Errorf("database driver '%s' does not exist", m.Database.Driver)
	}
	return nil
}

// Provision provisions t. Implements caddy.Provisioner.
func (m *XTemplateModule) Provision(ctx caddy.Context) error {
	log := slog.New(zapslog.NewHandler(ctx.Logger().Core(), nil))

	config := xtemplate.New()
	config.WithConfig(m.Config).WithLogger(log.WithGroup("xtemplate"))

	var watchDirs []string

	// Context FS
	if m.ContextRoot != "" {
		cfs := os.DirFS(m.ContextRoot)
		if st, err := fs.Stat(cfs, "."); err != nil || !st.IsDir() {
			return fmt.Errorf("context file path does not exist in filesystem: %v", err)
		}
		config.WithContextFS(cfs)
		watchDirs = append(watchDirs, m.ContextRoot)
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
		config.WithTemplateFS(tfs)
		watchDirs = append(watchDirs, root)
	}

	// ExtraFuncs
	{
		_, err := config.WithRegisteredFuncMaps(m.FuncsModules...)
		if err != nil {
			return err
		}
	}

	if m.Database.Driver != "" {
		db, err := sql.Open(m.Database.Driver, m.Database.Connstr)
		if err != nil {
			return err
		}
		config.WithDB(db)
		m.db = db
	}

	if len(m.Delimiters) != 0 {
		config.WithDelims(m.Delimiters[0], m.Delimiters[1])
	}

	{
		h, err := config.Build()
		if err != nil {
			return err
		}
		m.handler = h
	}

	if len(watchDirs) > 0 {
		changed, halt, err := watch.WatchDirs(watchDirs, 200*time.Millisecond)
		if err != nil {
			return err
		}
		m.halt = halt
		watch.React(changed, halt, func() (halt bool) {
			newhandler, err := config.Build()
			if err != nil {
				log.Info("failed to reload xtemplate", "error", err)
			} else {
				m.handler = newhandler
				log.Info("reloaded templates after file changed")
			}
			return
		})
	}
	return nil
}

func (m *XTemplateModule) ServeHTTP(w http.ResponseWriter, r *http.Request, _ caddyhttp.Handler) error {
	m.handler.ServeHTTP(w, r)
	return nil
}

// Cleanup discards resources held by t. Implements caddy.CleanerUpper.
func (m *XTemplateModule) Cleanup() error {
	if m.halt != nil {
		m.halt <- struct{}{}
		close(m.halt)
		m.halt = nil
	}
	if m.db != nil {
		err := m.db.Close()
		m.db = nil
		return err
	}
	return nil
}

// Interface guards
var (
	_ caddy.Validator             = (*XTemplateModule)(nil)
	_ caddy.Provisioner           = (*XTemplateModule)(nil)
	_ caddyhttp.MiddlewareHandler = (*XTemplateModule)(nil)
	_ caddy.CleanerUpper          = (*XTemplateModule)(nil)
)
