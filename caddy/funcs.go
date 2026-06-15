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

// validateFuncsModules checks, without instantiating anything that needs a
// caddy.Context, that each named module exists in the `xtemplate.funcs`
// namespace and implements FuncsProvider. It is intended to be called from
// Validate so misconfiguration is reported early.
func validateFuncsModules(names []string) error {
	for _, name := range names {
		id := funcsModuleID(name)
		mi, err := caddy.GetModule(id)
		if err != nil {
			return fmt.Errorf("failed to find module '%s': %w", id, err)
		}
		if _, ok := mi.New().(FuncsProvider); !ok {
			return fmt.Errorf("module '%s' does not implement FuncsProvider", id)
		}
	}
	return nil
}

// resolveFuncsModules looks up each named module in the `xtemplate.funcs`
// namespace, provisions it if it is a caddy.Provisioner, and collects the
// template.FuncMap each one provides.
func resolveFuncsModules(ctx caddy.Context, names []string) ([]template.FuncMap, error) {
	var funcMaps []template.FuncMap
	for _, name := range names {
		id := funcsModuleID(name)
		mi, err := caddy.GetModule(id)
		if err != nil {
			return nil, fmt.Errorf("failed to find module '%s': %w", id, err)
		}
		inst := mi.New()
		fp, ok := inst.(FuncsProvider)
		if !ok {
			return nil, fmt.Errorf("module '%s' does not implement FuncsProvider", id)
		}
		if prov, ok := inst.(caddy.Provisioner); ok {
			if err := prov.Provision(ctx); err != nil {
				return nil, fmt.Errorf("failed to provision module '%s': %w", id, err)
			}
		}
		funcMaps = append(funcMaps, fp.Funcs())
	}
	return funcMaps, nil
}
