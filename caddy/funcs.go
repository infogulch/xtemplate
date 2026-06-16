package xtemplate_caddy

import (
	"fmt"
	"html/template"

	"github.com/caddyserver/caddy/v2"
)

// FuncsProvider is implemented by Caddy modules in the `xtemplate.funcs`
// namespace. Such a module contributes additional functions to the xtemplate
// template execution context. Reference one by its module name (the final
// segment of its module ID) in the `funcs_modules` config field.
type FuncsProvider interface {
	Funcs() template.FuncMap
}

// funcsModuleID returns the full Caddy module ID for a funcs module name.
func funcsModuleID(name string) string {
	return "xtemplate.funcs." + name
}

// resolveFuncsModules looks up each named module in the `xtemplate.funcs`
// namespace and returns a fresh, unprovisioned instance of each, verifying that
// it implements FuncsProvider. It needs no caddy.Context, so Validate can call
// it to report misconfiguration early; Provision calls it and hands the result
// to provisionFuncsModules.
func resolveFuncsModules(names []string) ([]FuncsProvider, error) {
	fps := make([]FuncsProvider, 0, len(names))
	for _, name := range names {
		id := funcsModuleID(name)
		mi, err := caddy.GetModule(id)
		if err != nil {
			return nil, fmt.Errorf("failed to find module '%s': %w", id, err)
		}
		fp, ok := mi.New().(FuncsProvider)
		if !ok {
			return nil, fmt.Errorf("module '%s' does not implement FuncsProvider", id)
		}
		fps = append(fps, fp)
	}
	return fps, nil
}

// provisionFuncsModules provisions each FuncsProvider that is also a
// caddy.Provisioner and collects the template.FuncMap each one provides.
func provisionFuncsModules(ctx caddy.Context, fps []FuncsProvider) ([]template.FuncMap, error) {
	funcMaps := make([]template.FuncMap, 0, len(fps))
	for _, fp := range fps {
		if prov, ok := fp.(caddy.Provisioner); ok {
			if err := prov.Provision(ctx); err != nil {
				id := ""
				if mod, ok := fp.(caddy.Module); ok {
					id = string(mod.CaddyModule().ID)
				}
				return nil, fmt.Errorf("failed to provision module '%s': %w", id, err)
			}
		}
		funcMaps = append(funcMaps, fp.Funcs())
	}
	return funcMaps, nil
}
