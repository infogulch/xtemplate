package xtemplate

import (
	"context"
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
	"slices"
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

// Instance is a configured, immutable, xtemplate request handler ready to
// execute templates and serve static files in response to http requests.
//
// The only way to create a valid Instance is to call the [Config.Instance]
// method. Configuration of an Instance is intended to be immutable. Instead of
// mutating a running Instance, build a new Instance from a modified Config and
// swap them.
//
// See also [Server] which manages instances and enables reloading them.
type Instance struct {
	config Config
	id     int64

	router    *http.ServeMux
	files     map[string]*fileInfo
	templates *template.Template
	funcs     template.FuncMap

	bufferDot  *dot
	flusherDot *dot
}

// Instance creates a new *xinstance from the given config
func (config Config) Instance() (*Instance, InstanceStats, []InstanceRoute, error) {
	start := time.Now()

	config.Defaults()
	inst := &Instance{
		config: config,
		id:     nextInstanceIdentity.Add(1),
	}

	build := &builder{
		Instance: inst,
	}

	inst.config.Logger = inst.config.Logger.With(slog.Int64("instance", inst.id))
	inst.config.Logger.Info("initializing")

	if inst.config.TemplatesFS == nil {
		inst.config.TemplatesFS = os.DirFS(inst.config.TemplatesDir)
	}

	{
		inst.funcs = template.FuncMap{}
		maps.Copy(inst.funcs, xtemplateFuncs)
		maps.Copy(inst.funcs, sprig.HtmlFuncMap())
		for _, extra := range inst.config.FuncMaps {
			maps.Copy(inst.funcs, extra)
		}
	}

	inst.files = make(map[string]*fileInfo)
	inst.router = http.NewServeMux()
	inst.templates = template.New(".").Delims(inst.config.LDelim, inst.config.RDelim).Funcs(inst.funcs)

	if config.Minify {
		m := minify.New()
		m.Add("text/css", &css.Minifier{})
		m.Add("image/svg+xml", &svg.Minifier{})
		m.Add("text/html", &html.Minifier{
			TemplateDelims: [...]string{inst.config.LDelim, inst.config.RDelim},
		})
		m.AddRegexp(regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"), &js.Minifier{})
		build.m = m
	}

	inst.bufferDot = makeDot(slices.Concat([]DotConfig{{"X", dotXProvider{inst}}, {"Req", dotReqProvider{}}}, config.Dot, []DotConfig{{"Resp", dotRespProvider{}}}))
	inst.flusherDot = makeDot(slices.Concat([]DotConfig{{"X", dotXProvider{inst}}, {"Req", dotReqProvider{}}}, config.Dot, []DotConfig{{"Flush", dotFlushProvider{}}}))

	if err := fs.WalkDir(inst.config.TemplatesFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if ext := filepath.Ext(path); ext == inst.config.TemplateExtension {
			err = build.addTemplateHandler(path)
		} else {
			err = build.addStaticFileHandler(path)
		}
		return err
	}); err != nil {
		return nil, InstanceStats{}, nil, fmt.Errorf("error scanning files: %w", err)
	}

	initDot := makeDot(append([]DotConfig{{"X", dotXProvider{inst}}}, config.Dot...))

	// Invoke all initilization templates, aka any template whose name starts with "INIT ".
	for _, tmpl := range inst.templates.Templates() {
		if strings.HasPrefix(tmpl.Name(), "INIT ") {
			val, err := initDot.value(inst.config.Logger, config.Ctx, nil, nil)
			if err != nil {
				return nil, InstanceStats{}, nil, fmt.Errorf("failed to get init dot value: %w", err)
			}
			err = tmpl.Execute(io.Discard, val)
			if err = initDot.cleanup(val, err); err != nil {
				return nil, InstanceStats{}, nil, fmt.Errorf("template initializer '%s' failed: %w", tmpl.Name(), err)
			}
			build.TemplateInitializers += 1
		}
	}

	inst.config.Logger.Info("instance loaded",
		slog.Duration("load_time", time.Since(start)),
		slog.Group("stats",
			slog.Int("routes", build.Routes),
			slog.Int("templateFiles", build.TemplateFiles),
			slog.Int("templateDefinitions", build.TemplateDefinitions),
			slog.Int("templateInitializers", build.TemplateInitializers),
			slog.Int("staticFiles", build.StaticFiles),
			slog.Int("staticFilesAlternateEncodings", build.StaticFilesAlternateEncodings),
		))

	return inst, build.InstanceStats, build.routes, nil
}

// Counter to assign a unique id to each instance of xtemplate created when
// calling Config.Instance(). This is intended to help distinguish logs from
// multiple instances in a single process.
var nextInstanceIdentity atomic.Int64

// Id returns the id of this instance which is unique in the current
// process. This differentiates multiple instances, as the instance id
// is attached to all logs generated by the instance with the attribute name
// `xtemplate.instance`.
func (x *Instance) Id() int64 {
	return x.id
}

var (
	levelDebug2 slog.Level = slog.LevelDebug + 2
)

func (instance *Instance) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case <-instance.config.Ctx.Done():
		instance.config.Logger.Error("received request after xtemplate instance cancelled", slog.String("method", r.Method), slog.String("path", r.URL.Path))
		http.Error(w, "server stopped", http.StatusInternalServerError)
		return
	default:
	}

	// See handlers.go
	handler, handlerPattern := instance.router.Handler(r)

	log := instance.config.Logger.With(slog.Group("serve",
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

// LoggerKey is the context value key used to smuggle the current logger through
// the http.Handler interface.
var LoggerKey = reflect.TypeOf((*slog.Logger)(nil))

func getCtxLogger(r *http.Request) *slog.Logger {
	log, ok := r.Context().Value(LoggerKey).(*slog.Logger)
	if !ok {
		return slog.Default()
	}
	return log
}
