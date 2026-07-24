package dotflags

import (
	"fmt"
	"net/http"

	"github.com/infogulch/xtemplate"
)

func init() {
	xtemplate.RegisterProvider("flags", func() xtemplate.Provider { return &DotFlagsConfig{} })
}

// DotFlags provides template access to a static key/value map.
type DotFlags struct {
	m map[string]string
}

func (d DotFlags) Value(key string) string {
	return d.m[key]
}

// WithFlags creates an [xtemplate.Option] that adds a flags dot provider to
// the config.
func WithFlags(name string, flags map[string]string) xtemplate.Option {
	return func(c *xtemplate.Config) error {
		if flags == nil {
			return fmt.Errorf("cannot create DotFlagsProvider with null map with name %s", name)
		}
		c.Providers = append(c.Providers, &DotFlagsConfig{name, flags})
		return nil
	}
}

// DotFlagsConfig configures an xtemplate dot field to expose a static
// key/value map to templates.
type DotFlagsConfig struct {
	Name   string            `json:"name"`
	Values map[string]string `json:"values"`
}

var _ xtemplate.Provider = &DotFlagsConfig{}

func (d *DotFlagsConfig) FieldName() string { return d.Name }
func (d *DotFlagsConfig) Prototype() any    { return DotFlags{} }
func (d *DotFlagsConfig) Value(http.ResponseWriter, *http.Request) (any, error) {
	return DotFlags{d.Values}, nil
}
