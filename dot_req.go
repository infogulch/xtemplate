package xtemplate

import (
	"net/http"
)

type dotReqProvider struct{}

func (dotReqProvider) Value(r Request) (any, error) {
	return DotReq{r.R}, nil
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
