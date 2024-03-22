package providers

import (
	"context"
	"log/slog"
	"net/http"
	"reflect"

	"github.com/infogulch/xtemplate"
)

func init() {
	xtemplate.RegisterDot(&KVDot{})
}

func WithKV(name string, kv map[string]string) xtemplate.ConfigOverride {
	return xtemplate.WithProvider(name, KVDot{kv})
}

type KVDot struct {
	Map map[string]string
}

func (KVDot) New() xtemplate.DotProvider { return KVDot{} }
func (KVDot) Name() string               { return "kv" }
func (KVDot) Type() reflect.Type         { return reflect.TypeOf(KVDot{}) }

func (c KVDot) Value(log *slog.Logger, sctx context.Context, w http.ResponseWriter, r *http.Request) (reflect.Value, error) {
	return reflect.ValueOf(c), nil
}

func (c KVDot) Config(key string) string {
	return c.Map[key]
}
