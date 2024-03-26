package xtemplate

import (
	"context"
	"log/slog"
	"net/http"
	"reflect"
)

type dotReqProvider struct{}

func (dotReqProvider) Type() reflect.Type { return reflect.TypeOf(DotReq{}) }

func (dotReqProvider) Value(log *slog.Logger, sctx context.Context, w http.ResponseWriter, r *http.Request) (reflect.Value, error) {
	return reflect.ValueOf(DotReq{r}), nil
}

var _ DotProvider = dotReqProvider{}

type DotReq struct {
	*http.Request
}
