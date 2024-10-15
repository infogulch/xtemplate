package xtemplate

import (
	"context"
	"fmt"
)

type DotFlags struct {
	m map[string]string
}

func (d DotFlags) Value(key string) string {
	return d.m[key]
}

func WithFlags(name string, flags map[string]string) Option {
	return func(c *Config) error {
		if flags == nil {
			return fmt.Errorf("cannot create DotKVProvider with null map with name %s", name)
		}
		c.Flags = append(c.Flags, DotFlagsConfig{name, flags})
		return nil
	}
}

type DotFlagsConfig struct {
	Name   string            `json:"name"`
	Values map[string]string `json:"values"`
}

var _ DotConfig = &DotFlagsConfig{}

func (d *DotFlagsConfig) FieldName() string            { return d.Name }
func (d *DotFlagsConfig) Init(_ context.Context) error { return nil }
func (d *DotFlagsConfig) Value(_ Request) (any, error) { return DotFlags{d.Values}, nil }
