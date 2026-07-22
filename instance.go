package xtemplate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/felixge/httpsnoop"
	"github.com/spf13/afero"

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
	id      int64
	handler http.Handler

	config Config

	files     map[string]*fileInfo
	templates *template.Template
	funcs     template.FuncMap

	bufferDot  dot
	flusherDot dot

	closeOnce func() error

	// inflight counts requests that have entered ServeHTTP and not yet returned.
	// Enter is add-then-check-cancel (no mutex); see waitInFlight.
	inflight atomic.Int64
}

// Instance creates a new *Instance from the given config and options.
func (config *Config) Instance(cfgs ...Option) (_ *Instance, _ *InstanceStats, _ []InstanceRoute, err error) {
	start := time.Now()

	inst := &Instance{
		config: *config.SetDefaults(),
		id:     nextInstanceIdentity.Add(1),
	}
	// clone onClose so Options cannot trample the caller's base config
	inst.config.onClose = slices.Clone(inst.config.onClose)
	inst.closeOnce = sync.OnceValue(func() (err error) {
		for _, fn := range slices.Backward(inst.config.onClose) {
			err = errors.Join(err, fn())
		}
		return
	})
	defer func() {
		if err != nil {
			err = errors.Join(err, inst.closeOnce())
		}
	}()

	if _, err = inst.config.Options(cfgs...); err != nil {
		return nil, nil, nil, err
	}

	inst.config.Logger = inst.config.Logger.With(slog.Int64("instance", inst.id))
	inst.config.Logger.Info("initializing")
	inst.files = make(map[string]*fileInfo)

	if inst.config.TemplatesFS == nil {
		inst.config.TemplatesFS = afero.NewBasePathFs(afero.NewOsFs(), inst.config.TemplatesDir)
	}

	if len(inst.config.Precompress) > 0 {
		for _, encoding := range inst.config.Precompress {
			if _, ok := encodingExts[encoding]; !ok {
				return nil, nil, nil, fmt.Errorf("unsupported encoding: %s", encoding)
			}
		}
		tempdir, err := os.MkdirTemp("", "xtemplate-precompress-*")
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create temp dir for pre-compressed files: %w", err)
		}
		go func() {
			<-inst.config.Ctx.Done()
			_ = os.RemoveAll(tempdir)
		}()
		overlay := afero.NewBasePathFs(afero.NewOsFs(), tempdir)
		inst.config.TemplatesFS = afero.NewCopyOnWriteFs(inst.config.TemplatesFS, overlay)
	}

	{
		inst.funcs = template.FuncMap{}
		maps.Copy(inst.funcs, xtemplateFuncs)
		maps.Copy(inst.funcs, sprig.HtmlFuncMap())
		for _, extra := range inst.config.FuncMaps {
			maps.Copy(inst.funcs, extra)
		}
	}
	// Funcs must be installed before any parse trees are added to the set.
	inst.templates = template.New(".").Delims(inst.config.LDelim, inst.config.RDelim).Funcs(inst.funcs)

	build := &builder{
		Instance:      inst,
		InstanceStats: &InstanceStats{},
		router:        http.NewServeMux(),
	}

	if build.config.Minify != nil && *build.config.Minify {
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
		if err != nil {
			return err
		}
		// Don't walk hidden dirs (e.g. .git) which often hold sensitive or junk files.
		if d.IsDir() {
			if name := d.Name(); name != "." && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
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

	for _, route := range build.config.Handlers {
		pattern, handler := route.Pattern, route.Handler
		if err := catch(fmt.Sprintf("add custom handler to servemux '%s'", pattern), func() { build.router.Handle(pattern, handler) }); err != nil {
			return nil, nil, nil, err
		}
		build.routes = append(build.routes, InstanceRoute{pattern, handler})
		build.Routes += 1
	}

	dcInstance := dotXProvider{build.Instance}
	dcReq := dotReqProvider{}
	dcResp := dotRespProvider{}
	dcFlush := &dotFlushProvider{}

	var dot []Provider

	{
		var err error
		if dot, err = resolveProviders(build.config.ProvidersRaw); err != nil {
			return nil, nil, nil, err
		}
		dot = append(dot, build.config.Providers...)
		seen := map[string]bool{}
		for _, d := range dot {
			name := d.FieldName()
			if seen[name] {
				return nil, nil, nil, fmt.Errorf("dot field name '%s' is used more than once", name)
			}
			seen[name] = true
		}

		// Process all providers for initialization and closing
		for _, d := range slices.Concat([]Provider{dcInstance, dcReq, dcResp, dcFlush}, dot) {
			if di, ok := d.(Initializer); ok {
				start := time.Now()
				err := di.Init(build.config.Ctx)
				build.config.Logger.Debug("initialized provider", "name", d.FieldName(), "type", reflect.TypeOf(d).String(), "duration", time.Since(start))
				if err != nil {
					return nil, nil, nil, fmt.Errorf("failed to initialize dot field '%s': %w", d.FieldName(), err)
				}
			}
			// Any provider that implements Closer is added to the close list after initialization
			if dc, ok := d.(Closer); ok {
				inst.config.onClose = append(inst.config.onClose, func() error {
					err := dc.Close()
					if err != nil {
						err = fmt.Errorf("failed to close provider. type=%T, field=%s: %w", d, d.FieldName(), err)
					}
					return err
				})
			}
		}
	}

	inst.config.onClose = append(inst.config.onClose, func() error {
		if n := inst.inflight.Load(); n > 0 {
			inst.config.Logger.Warn("closing instance with unresolved requests",
				slog.Int64("instance", inst.id),
				slog.Int64("inflight", n),
			)
		}
		return nil
	})

	if build.bufferDot, err = makeDot(slices.Concat([]Provider{dcInstance, dcReq}, dot, []Provider{dcResp})); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to build buffer dot: %w", err)
	}
	if build.flusherDot, err = makeDot(slices.Concat([]Provider{dcInstance, dcReq}, dot, []Provider{dcFlush})); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to build flusher dot: %w", err)
	}

	{
		// Invoke all initialization templates, aka any template whose name starts
		// with "INIT ".
		makeDot := func() (*reflect.Value, error) {
			w, r := httptest.NewRecorder(), httptest.NewRequest("", "/", nil)
			return build.bufferDot.value(w, r)
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
					return nil, nil, nil, fmt.Errorf("initialization template '%s' failed: %w", tmpl.Name(), err)
				}
				// TODO: output buffer somewhere?
				build.config.Logger.Debug("executed initialization template", slog.String("template_name", tmpl.Name()), slog.Int("rendered_len", buf.Len()))
				build.InitializationTemplates += 1
			}
		}
	}

	build.config.Logger.Info("instance loaded",
		slog.Duration("load_time", time.Since(start)),
		slog.Group("stats",
			slog.Int("routes", build.Routes),
			slog.Int("templateFiles", build.TemplateFiles),
			slog.Int("templateDefinitions", build.TemplateDefinitions),
			slog.Int("initializationTemplates", build.InitializationTemplates),
			slog.Int("staticFiles", build.StaticFiles),
			slog.Int("staticFilesAlternateEncodings", build.StaticFilesAlternateEncodings),
		))

	if config.CrossOrigin.Disabled {
		build.handler = build.router
	} else {
		handler := http.NewCrossOriginProtection()
		for _, origin := range config.CrossOrigin.TrustedOrigins {
			_ = handler.AddTrustedOrigin(origin)
		}
		for _, pattern := range config.CrossOrigin.InsecureBypassPatterns {
			handler.AddInsecureBypassPattern(pattern)
		}
		build.handler = handler.Handler(build.router)
	}

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

// Close releases instance-scoped provider resources ([Closer]), then runs
// [WithOnClose] callbacks. Invocations after the first return a memoized result
// and do not call [Closer.Close] or [WithOnClose] callbacks again.
//
// If requests are still in flight (grace period expired or Stop skipped drain),
// Close logs this as a warning.
func (x *Instance) Close() error {
	if x == nil {
		return nil
	}
	return x.closeOnce()
}

var levelDebug2 slog.Level = slog.LevelDebug + 2

func (instance *Instance) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Add before cancel check ensures waitInFlight cannot observe inflight=0.
	instance.inflight.Add(1)
	if instance.config.Ctx.Err() != nil {
		instance.inflight.Add(-1)
		instance.config.Logger.Error("received request after xtemplate instance cancelled", slog.String("method", r.Method), slog.String("path", r.URL.Path))
		http.Error(w, "server stopped", http.StatusInternalServerError)
		return
	}
	defer instance.inflight.Add(-1)

	ctx := r.Context()
	rid := GetRequestId(ctx)
	if rid == "" {
		id, err := uuid.NewV7()
		if err != nil {
			instance.config.Logger.Error("failed to generate request id", slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.Any("error", err))
			http.Error(w, "failed to generate request id", http.StatusInternalServerError)
			return
		}
		rid = id.String()
		ctx = context.WithValue(ctx, requestIdKey, rid)
	}

	log := instance.config.Logger.With(slog.Group("serve",
		slog.String("requestid", rid),
	))
	log.LogAttrs(ctx, slog.LevelDebug, "serving request",
		slog.String("user-agent", r.Header.Get("User-Agent")),
		slog.String("method", r.Method),
		slog.String("requestPath", r.URL.Path),
	)
	ctx = context.WithValue(ctx, loggerKey, log)

	r = r.WithContext(ctx)
	metrics := httpsnoop.CaptureMetrics(instance.handler, w, r)

	log.LogAttrs(ctx, levelDebug2, "request served",
		slog.Group("response",
			slog.Duration("duration", metrics.Duration),
			slog.Int("statusCode", metrics.Code),
			slog.Int64("bytes", metrics.Written),
			slog.String("pattern", r.Pattern),
		))
}

// waitInFlight blocks until inflight is 0 or ctx is done. Caller must cancel
// the instance context first: concurrent enters then bump the counter, see
// cancel, and decrement without running the handler. Retire is rare, so a
// short poll is enough (returns immediately when already idle).
func (instance *Instance) waitInFlight(ctx context.Context) {
	if instance == nil {
		return
	}
	if instance.inflight.Load() == 0 {
		return
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if instance.inflight.Load() == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
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
