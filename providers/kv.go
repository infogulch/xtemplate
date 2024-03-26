package providers

import (
	"context"
	"log/slog"
	"net/http"
	"reflect"

	"github.com/infogulch/xtemplate"
)

func init() {
	xtemplate.RegisterDot(&DotKVProvider{})
}

func WithKV(name string, kv map[string]string) xtemplate.ConfigOverride {
	return xtemplate.WithProvider(name, DotKVProvider{kv})
}

type DotKVProvider struct {
	Map map[string]string
}

func (DotKVProvider) New() xtemplate.DotProvider { return &DotKVProvider{} }
func (DotKVProvider) Name() string               { return "kv" }
func (DotKVProvider) Type() reflect.Type         { return reflect.TypeOf(DotKV{}) }

func (c DotKVProvider) Value(log *slog.Logger, sctx context.Context, w http.ResponseWriter, r *http.Request) (reflect.Value, error) {
	return reflect.ValueOf(DotKV{c.Map}), nil
}

type DotKV struct {
	m map[string]string
}

func (d DotKV) Value(key string) string {
	return d.m[key]
}
