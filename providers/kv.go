package providers

import (
	"fmt"

	"github.com/infogulch/xtemplate"
)

func init() {
	xtemplate.RegisterDot(&DotKVProvider{})
}

func WithKV(name string, kv map[string]string) xtemplate.ConfigOverride {
	if kv == nil {
		panic(fmt.Sprintf("cannot create DotKVProvider with null map with name %s", name))
	}
	return xtemplate.WithProvider(name, &DotKVProvider{kv})
}

type DotKVProvider struct {
	Values map[string]string `json:"values"`
}

func (DotKVProvider) New() xtemplate.DotProvider { return &DotKVProvider{} }
func (DotKVProvider) Type() string               { return "kv" }

func (c *DotKVProvider) Value(xtemplate.Request) (any, error) {
	if c.Values == nil {
		c.Values = make(map[string]string)
	}
	return DotKV{c.Values}, nil
}

type DotKV struct {
	m map[string]string
}

func (d DotKV) Value(key string) string {
	return d.m[key]
}
