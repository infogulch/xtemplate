package xtemplate

import (
	"net/http"
)

type dotReqProvider struct{}

func (dotReqProvider) Value(r Request) (any, error) {
	return DotReq{r.R}, nil
}

var _ DotProvider = dotReqProvider{}

// DotReq is used as the .Req field for template invocations with an associated
// request, and contains the current HTTP request struct which can be used to
// read request data. See [http.Request] for detailed documentation. Some
// notable methods and fields:
//
// [http.Request.Method], [http.Request.PathValue], [http.Request.URL],
// [http.Request.URL.Query], [http.Request.Cookie], [http.Request.Header],
// [http.Request.Header.Get].
//
// Note that [http.Request.ParseForm] must be called before using
// [http.Request.Form], [http.Request.PostForm], and [http.Request.PostValue].
type DotReq struct {
	*http.Request
}
