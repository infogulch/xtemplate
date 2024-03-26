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

// DotReq is used as the `.Req` field for template invocations with an
// associated request, and contains the current HTTP request struct which can be
// used to read the detailed contents of the request. See [http.Request] for
// detailed documentation. Some notable methods and fields:
//
// `Method`, `PathValue`, `URL`, `URL.Query`, `URL.Host`, `URL.Path`, `Cookie`
// `Header`, `Header.Get`.
//
// Note that `ParseForm` must be called before using `Form`, `PostForm`, and
// `PostValue`.
type DotReq struct {
	*http.Request
}
