package xtemplate

import (
	"database/sql"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"

	"github.com/Masterminds/sprig/v3"
	"github.com/infogulch/xtemplate/internal"
)

func New() *config {
	c := &config{}
	c.WithFuncMaps(xtemplateFuncs, sprig.HtmlFuncMap())
	return c
}

type config []func(*xtemplate)

func (c *config) WithTemplateFS(tfs fs.FS) *config {
	*c = append(*c, func(r *xtemplate) {
		r.templateFS = tfs
	})
	return c
}

func (c *config) WithRegisteredTemplateFS(name string) (*config, error) {
	cfs, ok := internal.RegisteredFS[name]
	if !ok {
		return c, fmt.Errorf("fs named '%s' not registered", name)
	}
	c.WithContextFS(cfs)
	return c, nil
}

func (c *config) WithContextFS(cfs fs.FS) *config {
	*c = append(*c, func(r *xtemplate) {
		r.contextFS = cfs
	})
	return c
}

func (c *config) WithRegisteredContextFS(name string) (*config, error) {
	cfs, ok := internal.RegisteredFS[name]
	if !ok {
		return c, fmt.Errorf("fs named '%s' not registered", name)
	}
	c.WithContextFS(cfs)
	return c, nil
}

func (c *config) WithFuncMaps(funcmaps ...template.FuncMap) *config {
	for _, funcs := range funcmaps {
		funcs := funcs
		*c = append(*c, func(r *xtemplate) {
			for name, fn := range funcs {
				r.funcs[name] = fn
			}
		})
	}
	return c
}

func (c *config) WithRegisteredFuncMaps(names ...string) (*config, error) {
	for _, name := range names {
		funcs, ok := internal.RegisteredFuncMaps[name]
		if !ok {
			return c, fmt.Errorf("funcmap named '%s' not registered", name)
		}
		c.WithFuncMaps(funcs)
	}
	return c, nil
}

func (c *config) WithAllRegisteredFuncMaps() *config {
	for _, m := range internal.RegisteredFuncMaps {
		c.WithFuncMaps(m)
	}
	return c
}

func (c *config) WithDB(db *sql.DB) *config {
	*c = append(*c, func(r *xtemplate) {
		r.db = db
	})
	return c
}

func (c *config) WithDelims(l, r string) *config {
	*c = append(*c, func(rt *xtemplate) {
		rt.ldelim = l
		rt.rdelim = r
	})
	return c
}

func (c *config) WithConfig(cfg map[string]string) *config {
	*c = append(*c, func(r *xtemplate) {
		for k, v := range cfg {
			r.config[k] = v
		}
	})
	return c
}

func (c *config) WithLogger(log *slog.Logger) *config {
	*c = append(*c, func(r *xtemplate) {
		r.log = log
	})
	return c
}

// Call config.Build() to get an http.Handler that can handle http requests
