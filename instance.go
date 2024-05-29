package xtemplate

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/felixge/httpsnoop"
	"github.com/google/uuid"
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

	bufferDot  dot
	flusherDot dot
}

// Instance creates a new *Instance from the given config
func (config Config) Instance(cfgs ...Option) (*Instance, *InstanceStats, []InstanceRoute, error) {
	start := time.Now()

	if _, err := config.Defaults().Options(cfgs...); err != nil {
		return nil, nil, nil, err
	}

	build := &builder{
		Instance: &Instance{
			config: config,
			id:     nextInstanceIdentity.Add(1),
		},
		InstanceStats: &InstanceStats{},
	}

	build.config.Logger = build.config.Logger.With(slog.Int64("instance", build.id))
	build.config.Logger.Info("initializing")

	if build.config.TemplatesFS == nil {
		build.config.TemplatesFS = os.DirFS(build.config.TemplatesDir)
	}

	{
		build.funcs = template.FuncMap{}
		maps.Copy(build.funcs, xtemplateFuncs)
		maps.Copy(build.funcs, sprig.HtmlFuncMap())
		for _, extra := range build.config.FuncMaps {
			maps.Copy(build.funcs, extra)
		}
	}

	build.files = make(map[string]*fileInfo)
	build.router = http.NewServeMux()
	build.templates = template.New(".").Delims(build.config.LDelim, build.config.RDelim).Funcs(build.funcs)

	if config.Minify {
		m := minify.New()
		m.Add("text/css", &css.Minifier{})
		m.Add("image/svg+xml", &svg.Minifier{})
		m.Add("text/html", &html.Minifier{
			TemplateDelims: [...]string{build.config.LDelim, build.config.RDelim},
		})
		m.AddRegexp(regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"), &js.Minifier{})
		build.m = m
	}

	if err := fs.WalkDir(build.config.TemplatesFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasSuffix(path, build.config.TemplateExtension) {
			err = build.addTemplateHandler(path)
		} else {
			err = build.addStaticFileHandler(path)
		}
		return err
	}); err != nil {
		return nil, nil, nil, fmt.Errorf("error scanning files: %w", err)
	}

	dcInstance := DotConfig{"X", "instance", dotXProvider{build.Instance}}
	dcReq := DotConfig{"Req", "req", dotReqProvider{}}
	dcResp := DotConfig{"Resp", "resp", dotRespProvider{}}
	dcFlush := DotConfig{"Flush", "flush", dotFlushProvider{}}

	build.bufferDot = makeDot(slices.Concat([]DotConfig{dcInstance, dcReq}, config.Dot, []DotConfig{dcResp}))
	build.flusherDot = makeDot(slices.Concat([]DotConfig{dcInstance, dcReq}, config.Dot, []DotConfig{dcFlush}))

	{
		// Invoke all initilization templates, aka any template whose name starts
		// with "INIT ".
		makeDot := func() (*reflect.Value, error) {
			w, r := httptest.NewRecorder(), httptest.NewRequest("", "/", nil)
			return build.bufferDot.value(config.Ctx, w, r)
		}
		cleanup := build.bufferDot.cleanup
		if val, err := makeDot(); err != nil { // run at least once
			return nil, nil, nil, fmt.Errorf("failed to initialize dot value: %w", err)
		} else if err = cleanup(val, err); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to cleanup test dot value: %w", err)
		}
		buf := new(bytes.Buffer)
		for _, tmpl := range build.templates.Templates() {
			buf.Reset()
			if strings.HasPrefix(tmpl.Name(), "INIT ") {
				val, err := makeDot()
				if err != nil {
					return nil, nil, nil, fmt.Errorf("failed to initialize dot value: %w", err)
				}
				err = tmpl.Execute(buf, *val)
				if err = cleanup(val, err); err != nil {
					return nil, nil, nil, fmt.Errorf("template initializer '%s' failed: %w", tmpl.Name(), err)
				}
				// TODO: output buffer somewhere?
				build.config.Logger.Debug("executed initializer", slog.String("template_name", tmpl.Name()), slog.Int("rendered_len", buf.Len()))
				build.TemplateInitializers += 1
			}
		}
	}

	build.config.Logger.Info("instance loaded",
		slog.Duration("load_time", time.Since(start)),
		slog.Group("stats",
			slog.Int("routes", build.Routes),
			slog.Int("templateFiles", build.TemplateFiles),
			slog.Int("templateDefinitions", build.TemplateDefinitions),
			slog.Int("templateInitializers", build.TemplateInitializers),
			slog.Int("staticFiles", build.StaticFiles),
			slog.Int("staticFilesAlternateEncodings", build.StaticFilesAlternateEncodings),
		))

	return build.Instance, build.InstanceStats, build.routes, nil
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

	ctx := r.Context()
	rid := GetRequestId(ctx)
	if rid == "" {
		rid = uuid.NewString()
		ctx = context.WithValue(ctx, requestIdKey, rid)
	}

	log := instance.config.Logger.With(slog.Group("serve",
		slog.String("requestid", rid),
	))
	log.LogAttrs(r.Context(), slog.LevelDebug, "serving request",
		slog.String("user-agent", r.Header.Get("User-Agent")),
		slog.String("method", r.Method),
		slog.String("requestPath", r.URL.Path),
	)
	ctx = context.WithValue(ctx, loggerKey, log)

	r = r.WithContext(ctx)
	metrics := httpsnoop.CaptureMetrics(instance.router, w, r)

	log.LogAttrs(r.Context(), levelDebug2, "request served",
		slog.Group("response",
			slog.Duration("duration", metrics.Duration),
			slog.Int("statusCode", metrics.Code),
			slog.Int64("bytes", metrics.Written),
			// Uncomment after release with this commit: https://github.com/golang/go/commit/a523152ea1df8d39d923ed90d19662896eff0607
			// slog.String("pattern", r.Pattern),
		))
}

type requestIdType struct{}

var requestIdKey = requestIdType{}

func GetRequestId(ctx context.Context) string {
	// xtemplate request id
	if av := ctx.Value(requestIdKey); av != nil {
		if v, ok := av.(string); ok {
			return v
		}
	}
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
	return ""
}

type loggerType struct{}

var loggerKey = loggerType{}

func GetLogger(ctx context.Context) *slog.Logger {
	log, ok := ctx.Value(loggerKey).(*slog.Logger)
	if !ok {
		return slog.Default()
	}
	return log
}
