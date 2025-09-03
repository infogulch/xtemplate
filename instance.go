package xtemplate

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/felixge/httpsnoop"
	"github.com/spf13/afero"

	"github.com/google/uuid"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go/jetstream"
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

	natsServer *server.Server
	natsClient *jetstream.JetStream

	bufferDot  dot
	flusherDot dot
}

// Instance creates a new *Instance from the given config
func (config *Config) Instance(cfgs ...Option) (*Instance, *InstanceStats, []InstanceRoute, error) {
	start := time.Now()

	build := &builder{
		Instance: &Instance{
			config: *config.Defaults(),
			id:     nextInstanceIdentity.Add(1),
		},
		InstanceStats: &InstanceStats{},
	}

	if _, err := build.config.Options(cfgs...); err != nil {
		return nil, nil, nil, err
	}

	build.config.Logger = build.config.Logger.With(slog.Int64("instance", build.id))
	build.config.Logger.Info("initializing")

	if build.config.TemplatesFS == nil {
		build.config.TemplatesFS = afero.NewBasePathFs(afero.NewOsFs(), build.config.TemplatesDir)
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

	if err := afero.Walk(build.config.TemplatesFS, ".", func(path_ string, d fs.FileInfo, err error) error {
		path_ = strings.ReplaceAll(path_, "\\", "/")
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasSuffix(path_, build.config.TemplateExtension) {
			err = build.addTemplateHandler(path_)
		} else {
			err = build.addStaticFileHandler(path_)
		}
		return err
	}); err != nil {
		return nil, nil, nil, fmt.Errorf("error scanning files: %w", err)
	}

	dcInstance := dotXProvider{build.Instance}
	dcReq := dotReqProvider{}
	dcResp := dotRespProvider{}
	dcFlush := dotFlushProvider{}

	var dot []DotConfig

	{
		names := map[string]int{}
		for _, d := range build.config.Databases {
			dot = append(dot, &d)
			names[d.FieldName()] += 1
		}
		for _, d := range build.config.Flags {
			dot = append(dot, &d)
			names[d.FieldName()] += 1
		}
		for _, d := range build.config.Directories {
			dot = append(dot, &d)
			names[d.FieldName()] += 1
		}
		for _, d := range build.config.Nats {
			dot = append(dot, &d)
			names[d.FieldName()] += 1
		}
		for _, d := range build.config.CustomProviders {
			dot = append(dot, d)
			names[d.FieldName()] += 1
		}
		for name, count := range names {
			if count > 1 {
				return nil, nil, nil, fmt.Errorf("dot field name '%s' is used %d times", name, count)
			}
		}
		for _, d := range dot {
			err := d.Init(build.config.Ctx)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("failed to initialize dot field '%s': %w", d.FieldName(), err)
			}
		}
	}

	build.bufferDot = makeDot(slices.Concat([]DotConfig{dcInstance, dcReq}, dot, []DotConfig{dcResp}))
	build.flusherDot = makeDot(slices.Concat([]DotConfig{dcInstance, dcReq}, dot, []DotConfig{dcFlush}))

	{
		// Invoke all initilization templates, aka any template whose name starts
		// with "INIT ".
		makeDot := func() (*reflect.Value, error) {
			w, r := httptest.NewRecorder(), httptest.NewRequest("", "/", nil)
			return build.bufferDot.value(build.config.Ctx, w, r)
		}
		cleanup := build.bufferDot.cleanup
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

func must[V any](v V, err error) V {
	if err != nil {
		log.Fatal(err)
	}
	return v
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
