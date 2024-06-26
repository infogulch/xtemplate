package providers

import (
	"fmt"

	"github.com/infogulch/xtemplate"
)

func init() {
	xtemplate.RegisterDot(&DotKVProvider{})
}

func WithKV(name string, kv map[string]string) xtemplate.Option {
	return func(c *xtemplate.Config) error {
		if kv == nil {
			return fmt.Errorf("cannot create DotKVProvider with null map with name %s", name)
		}
		return xtemplate.WithProvider(name, &DotKVProvider{kv})(c)
	}
}

type DotKVProvider struct {
	Values map[string]string `json:"values"`
}

var _ xtemplate.DotProvider = &DotKVProvider{}

func (DotKVProvider) New() xtemplate.DotProvider { return &DotKVProvider{} }
func (DotKVProvider) Type() string               { return "kv" }
func (c *DotKVProvider) Value(xtemplate.Request) (any, error) {
	if c.Values == nil {
		c.Values = make(map[string]string)
	}
	return DotKV{c.Values}, nil
}
