package xtemplate_caddy

import (
	"context"
	"net/http"
	"time"

	"log/slog"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/infogulch/watch"
	"github.com/infogulch/xtemplate"
	"go.uber.org/zap/exp/zapslog"
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
	xtemplate.Config

	WatchTemplatePath bool `json:"watch_template_path"`

	FuncsModules []string `json:"funcs_modules,omitempty"`

	handler http.Handler
	cancel  func()
}

// Validate ensures t has a valid configuration. Implements caddy.Validator.
func (m *XTemplateModule) Validate() error {
	// todo: process FuncsModules
	return nil
}

// Provision provisions t. Implements caddy.Provisioner.
func (m *XTemplateModule) Provision(ctx caddy.Context) error {
	// Wrap zap logger into a slog logger for xtemplate
	log := slog.New(zapslog.NewHandler(ctx.Logger().Core())).WithGroup("xtemplate-caddy")

	m.Logger = log
	m.Config.Defaults()
	m.Config.Ctx, m.cancel = context.WithCancel(ctx.Context)

	server, err := m.Config.Server()
	if err != nil {
		m.cancel()
		return err
	}
	m.handler = server.Handler()

	if m.WatchTemplatePath {
		halt, err := watch.Watch([]string{m.TemplatesDir}, 200*time.Millisecond, log.WithGroup("fswatch"), func() bool {
			err := server.Reload()
			if err != nil {
				log.Error("failed to reload xtemplate server", slog.Any("reload_error", err))
			}
			return true
		})
		if err != nil {
			return err
		}
		cancel := m.cancel
		m.cancel = func() {
			close(halt)
			if cancel != nil {
				cancel()
			}
		}
	}
	return nil
}

func (m *XTemplateModule) ServeHTTP(w http.ResponseWriter, r *http.Request, _ caddyhttp.Handler) error {
	m.handler.ServeHTTP(w, r)
	return nil
}

// Cleanup discards resources held by t. Implements caddy.CleanerUpper.
func (m *XTemplateModule) Cleanup() error {
	m.cancel()
	return nil
}

// Interface guards
var (
	_ caddy.Validator             = (*XTemplateModule)(nil)
	_ caddy.Provisioner           = (*XTemplateModule)(nil)
	_ caddyhttp.MiddlewareHandler = (*XTemplateModule)(nil)
	_ caddy.CleanerUpper          = (*XTemplateModule)(nil)
)
