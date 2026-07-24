package xtemplate_caddy

import (
	"context"
	"encoding/json"
	"net/http"

	"log/slog"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
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

	FuncsModules []string `json:"funcs_modules,omitempty"`

	handler *xtemplate.Server
	cancel  func()
}

// UnmarshalJSON applies the ban-list probe then unmarshals into the module.
// REMOVE BEFORE 1.0: temporary hard-rejects for renamed Caddy knobs.
//
// Uses a method-less alias of Config so Config.UnmarshalJSON (ban-list) is not
// invoked on the full object as an embedded Unmarshaler — that would swallow
// module-only fields like funcs_modules.
func (m *XTemplateModule) UnmarshalJSON(data []byte) error {
	if err := xtemplate.CheckLegacyTemplateKeys(data); err != nil {
		return err
	}
	type plainConfig xtemplate.Config
	type moduleAlias struct {
		plainConfig
		FuncsModules []string `json:"funcs_modules,omitempty"`
	}
	var a moduleAlias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	m.Config = xtemplate.Config(a.plainConfig)
	m.FuncsModules = a.FuncsModules
	return nil
}

// Validate ensures t has a valid configuration. Implements caddy.Validator.
func (m *XTemplateModule) Validate() error {
	_, err := resolveFuncsModules(m.FuncsModules)
	return err
}

// Provision provisions t. Implements caddy.Provisioner.
// Default without a source block is os path templates (no ad-hoc watch).
func (m *XTemplateModule) Provision(ctx caddy.Context) error {
	// Wrap zap logger into a slog logger for xtemplate
	log := slog.New(zapslog.NewHandler(ctx.Logger().Core())).WithGroup("xtemplate-caddy")

	m.Logger = log
	m.SetDefaults()
	m.Ctx, m.cancel = context.WithCancel(ctx.Context)

	// Resolve any `xtemplate.funcs.*` modules into template FuncMaps and pass
	// them to the server as options so they're available to all templates.
	fps, err := resolveFuncsModules(m.FuncsModules)
	if err != nil {
		m.cancel()
		return err
	}
	funcMaps, err := provisionFuncsModules(ctx, fps)
	if err != nil {
		m.cancel()
		return err
	}
	var opts []xtemplate.Option
	if len(funcMaps) > 0 {
		opts = append(opts, xtemplate.WithFuncMaps(funcMaps...))
	}

	server, err := m.Server(opts...)
	if err != nil {
		m.cancel()
		return err
	}
	m.handler = server
	return nil
}

func (m *XTemplateModule) ServeHTTP(w http.ResponseWriter, r *http.Request, _ caddyhttp.Handler) error {
	m.handler.ServeHTTP(w, r)
	return nil
}

// Cleanup discards resources held by t. Implements caddy.CleanerUpper.
// Stops the xtemplate Server so instance providers and contexts tear down even
// though Caddy never calls Serve (handler-only embed).
func (m *XTemplateModule) Cleanup() error {
	if m.handler != nil {
		m.handler.Stop()
	}
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}

// Interface guards
var (
	_ caddy.Validator             = (*XTemplateModule)(nil)
	_ caddy.Provisioner           = (*XTemplateModule)(nil)
	_ caddyhttp.MiddlewareHandler = (*XTemplateModule)(nil)
	_ caddy.CleanerUpper          = (*XTemplateModule)(nil)
	_ json.Unmarshaler            = (*XTemplateModule)(nil)
)
