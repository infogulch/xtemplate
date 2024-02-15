// xtemplate extends Go's html/template to be capable enough to define an entire
// server-side application with just templates.
package xtemplate

import (
	"context"
	"database/sql"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
)

// See config.go and build.go for how to create an instance of xtemplate.

type xtemplate struct {
	id int64

	templateFS      fs.FS
	contextFS       fs.FS
	config          map[string]string
	funcs           template.FuncMap
	db              *sql.DB
	templates       *template.Template
	router          *http.ServeMux
	files           map[string]fileInfo
	ldelim          string
	rdelim          string
	templateExtension string

	log    *slog.Logger
	ctx    context.Context
	cancel func()
}

func (x *xtemplate) Cancel() {
	x.log.Info("xtemplate instance cancelled")
	x.cancel()
}

func (server *xtemplate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	server.router.ServeHTTP(w, r)
}

type CancelHandler interface {
	http.Handler
	Cancel()
}

var _ = (CancelHandler)((*xtemplate)(nil))
