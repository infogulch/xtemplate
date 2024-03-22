package xtemplate

import (
	"context"
	"log/slog"
	"net/http"
	"reflect"
)

type requestDotProvider struct{}

func (requestDotProvider) Type() reflect.Type { return reflect.TypeOf(requestDot{}) }

func (requestDotProvider) Value(log *slog.Logger, sctx context.Context, w http.ResponseWriter, r *http.Request) (reflect.Value, error) {
	return reflect.ValueOf(requestDot{r}), nil
}

var _ DotProvider = requestDotProvider{}

type requestDot struct {
	*http.Request
}
