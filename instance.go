package xtemplate

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/felixge/httpsnoop"
	"github.com/segmentio/ksuid"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	"github.com/tdewolff/minify/v2/svg"
)

///////////////////////
// Pubic Definitions //
///////////////////////

type InstanceStats struct {
	Routes                        int
	TemplateFiles                 int
	TemplateDefinitions           int
	TemplateInitializers          int
	StaticFiles                   int
	StaticFilesAlternateEncodings int
}

type InstanceRoute struct {
	Pattern string
	Handler http.Handler
}

type Instance interface {
	http.Handler
	Id() int64
	Stats() InstanceStats
	Routes() []InstanceRoute
}

/////////////
// Builder //
/////////////

// Instance creates a new xtemplate.Instance from the given config
func (config Config) Instance() (Instance, error) {
	return config.instance()
}

// Instance creates a new *xinstance from the given config
func (config Config) instance() (*xinstance, error) {
	start := time.Now()

	config.Defaults()
	inst := &xinstance{
		Config: config,
	}

	inst.id = nextInstanceIdentity.Add(1)
	inst.Logger = inst.Logger.With(slog.Int64("instance", inst.id))
	inst.Logger.Info("initializing")

	if inst.Template.FS == nil {
		inst.Template.FS = os.DirFS(inst.Template.Path)
	}

	if inst.Context.FS == nil && inst.Context.Path != "" {
		inst.Context.FS = os.DirFS(inst.Context.Path)
	}

	if inst.Database.DB == nil && inst.Database.Driver != "" {
		var err error
		inst.Database.DB, err = sql.Open(inst.Database.Driver, inst.Database.Connstr)
		if err != nil {
			return nil, fmt.Errorf("failed to open database: driver: '%s', connstr: '%s', err: '%v'", inst.Database.Driver, inst.Database.Connstr, err)
		}
		err = inst.Database.DB.Ping()
		if err != nil {
			return nil, fmt.Errorf("failed to ping database after opening it: driver: `%s`, connstr: `%s`", inst.Database.Driver, inst.Database.Connstr)
		}
	}

	{
		inst.funcs = template.FuncMap{}
		maps.Copy(inst.funcs, xtemplateFuncs)
		maps.Copy(inst.funcs, sprig.HtmlFuncMap())
		for _, extra := range inst.FuncMaps {
			maps.Copy(inst.funcs, extra)
		}
	}

	inst.UserConfig = maps.Clone(inst.UserConfig)
	inst.files = make(map[string]*fileInfo)
	inst.router = http.NewServeMux()
	inst.templates = template.New(".").Delims(inst.Template.Delimiters.Left, inst.Template.Delimiters.Right).Funcs(inst.funcs)

	if config.Template.Minify {
		m := minify.New()
		m.Add("text/css", &css.Minifier{})
		m.Add("image/svg+xml", &svg.Minifier{})
		m.Add("text/html", &html.Minifier{
			TemplateDelims: [...]string{inst.Template.Delimiters.Left, inst.Template.Delimiters.Right},
		})
		m.AddRegexp(regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"), &js.Minifier{})
		inst.minify = m
	}

	if err := fs.WalkDir(inst.Template.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if ext := filepath.Ext(path); ext == inst.Template.TemplateExtension {
			err = inst.addTemplateHandler(path)
		} else {
			err = inst.addStaticFileHandler(path)
		}
		return err
	}); err != nil {
		return nil, fmt.Errorf("error scanning files: %v", err)
	}

	// Invoke all initilization templates, aka any template whose name starts with "INIT ".
	for _, tmpl := range inst.templates.Templates() {
		if strings.HasPrefix(tmpl.Name(), "INIT ") {
			context := &struct {
				baseContext
				fsContext
			}{
				baseContext{
					server: inst,
					log:    inst.Logger,
				},
				fsContext{
					fs: inst.Context.FS,
				},
			}
			err := tmpl.Execute(io.Discard, context)
			if err = context.resolvePendingTx(err); err != nil {
				return nil, fmt.Errorf("template initializer '%s' failed: %w", tmpl.Name(), err)
			}
			inst.stats.TemplateInitializers += 1
		}
	}

	inst.Logger.Info("instance loaded",
		slog.Duration("load_time", time.Since(start)),
		slog.Group("stats",
			slog.Int("routes", inst.stats.Routes),
			slog.Int("templateFiles", inst.stats.TemplateFiles),
			slog.Int("templateDefinitions", inst.stats.TemplateDefinitions),
			slog.Int("templateInitializers", inst.stats.TemplateInitializers),
			slog.Int("staticFiles", inst.stats.StaticFiles),
			slog.Int("staticFilesAlternateEncodings", inst.stats.StaticFilesAlternateEncodings),
		))

	return inst, nil
}

////////////////////
// Implementation //
////////////////////

// Counter to assign a unique id to each instance of xtemplate created when
// calling Build(). This is intended to help distinguish logs from multiple
// instances in a single process.
var nextInstanceIdentity atomic.Int64

type xinstance struct {
	Config
	id  int64
	ctx context.Context

	stats  InstanceStats
	routes []InstanceRoute
	minify *minify.M

	funcs     template.FuncMap
	files     map[string]*fileInfo
	router    *http.ServeMux
	templates *template.Template
}

var _ Instance = (*xinstance)(nil)

func (x *xinstance) Id() int64 {
	return x.id
}

func (x *xinstance) Stats() InstanceStats {
	return x.stats
}

func (x *xinstance) Routes() []InstanceRoute {
	return x.routes
}

var (
	levelDebug2 slog.Level = slog.LevelDebug + 2
)

func (server *xinstance) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case <-server.ctx.Done():
		server.Logger.Error("received request after xtemplate instance cancelled", slog.String("method", r.Method), slog.String("path", r.URL.Path))
		http.Error(w, "server stopped", http.StatusInternalServerError)
		return
	default:
	}

	// See handlers.go
	handler, handlerPattern := server.router.Handler(r)

	log := server.Logger.With(slog.Group("serve",
		slog.String("requestid", getRequestId(r.Context())),
	))
	log.LogAttrs(r.Context(), slog.LevelDebug, "serving request",
		slog.String("user-agent", r.Header.Get("User-Agent")),
		slog.String("method", r.Method),
		slog.String("requestPath", r.URL.Path),
		slog.String("handlerPattern", handlerPattern),
	)

	r = r.WithContext(context.WithValue(r.Context(), LoggerKey, log))
	metrics := httpsnoop.CaptureMetrics(handler, w, r)

	log.LogAttrs(r.Context(), levelDebug2, "request served",
		slog.Group("response",
			slog.Duration("duration", metrics.Duration),
			slog.Int("statusCode", metrics.Code),
			slog.Int64("bytes", metrics.Written)))
}

func getRequestId(ctx context.Context) string {
	// caddy request id
	if v := ctx.Value("vars"); v != nil {
		if mv, ok := v.(map[string]any); ok {
			if anyrid, ok := mv["uuid"]; ok {
				if rid, ok := anyrid.(string); ok {
					return rid
				}
			}
		}
	}
	return ksuid.New().String()
}

var LoggerKey = reflect.TypeOf((*slog.Logger)(nil))

func getCtxLogger(r *http.Request) *slog.Logger {
	log, ok := r.Context().Value(LoggerKey).(*slog.Logger)
	if !ok {
		return slog.Default()
	}
	return log
}
