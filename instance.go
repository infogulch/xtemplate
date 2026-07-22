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

	// providers is the user/core provider list for this instance (not builtins).
	// Used to call [Closer.Close] when the instance is retired.
	providers []Provider

	// onClose from [WithOnClose]; owned reslice for this instance only.
	onClose []func() error

	// inflight counts requests that have entered ServeHTTP and not yet returned.
	// Enter is add-then-check-cancel (no mutex); see waitInFlight.
	inflight atomic.Int64
}

// Instance creates a new *Instance from the given config
func (config *Config) Instance(cfgs ...Option) (inst *Instance, stats *InstanceStats, routes []InstanceRoute, err error) {
	start := time.Now()

	build := &builder{
		Instance: &Instance{
			config: *config.SetDefaults(),
			id:     nextInstanceIdentity.Add(1),
		},
		InstanceStats: &InstanceStats{},
	}
	// Reslice so Options append (and other builds) cannot share/trample the
	// caller's or base config's onClose backing array.
	build.config.onClose = slices.Clone(build.config.onClose)

	defer func() {
		if err != nil {
			// Build failed: release any OnClose resources registered for this attempt.
			if closeErr := runOnClose(build.config.onClose); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
			return
		}
		if inst != nil {
			inst.onClose = build.config.onClose
			inst.config.onClose = nil
		}
	}()

	if _, err = build.config.Options(cfgs...); err != nil {
		return nil, nil, nil, err
	}

	build.config.Logger = build.config.Logger.With(slog.Int64("instance", build.id))
	build.config.Logger.Info("initializing")

	if build.config.TemplatesFS == nil {
		build.config.TemplatesFS = afero.NewBasePathFs(afero.NewOsFs(), build.config.TemplatesDir)
	}

	if len(build.config.Precompress) > 0 {
		for _, encoding := range build.config.Precompress {
			if _, ok := encodingExts[encoding]; !ok {
				return nil, nil, nil, fmt.Errorf("unsupported encoding: %s", encoding)
			}
		}
		tempdir, err := os.MkdirTemp("", "xtemplate-precompress-*")
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create temp dir for pre-compressed files: %w", err)
		}
		go func() {
			<-build.config.Ctx.Done()
			_ = os.RemoveAll(tempdir)
		}()
		overlay := afero.NewBasePathFs(afero.NewOsFs(), tempdir)
		build.config.TemplatesFS = afero.NewCopyOnWriteFs(build.config.TemplatesFS, overlay)
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
		// Init user/core providers and any builtin that implements Initializer
		// (e.g. Flush stores instance ctx for Sleep/WaitForServerStop).
		toInit := slices.Concat([]Provider{dcInstance, dcReq, dcResp, dcFlush}, dot)
		var opened []Provider
		for _, d := range toInit {
			idp, ok := d.(Initializer)
			if !ok {
				continue
			}
			start := time.Now()
			err := idp.Init(build.config.Ctx)
			build.config.Logger.Debug("initialized provider", "name", d.FieldName(), "type", reflect.TypeOf(d).String(), "duration", time.Since(start))
			if err != nil {
				_ = closeProviders(opened)
				return nil, nil, nil, fmt.Errorf("failed to initialize dot field '%s': %w", d.FieldName(), err)
			}
			opened = append(opened, d)
		}
		// Only user/core providers are closed on instance retire (builtins have
		// no owned resources beyond the instance itself).
		build.providers = dot
	}

	if build.bufferDot, err = makeDot(slices.Concat([]Provider{dcInstance, dcReq}, dot, []Provider{dcResp})); err != nil {
		_ = closeProviders(dot)
		return nil, nil, nil, fmt.Errorf("failed to build buffer dot: %w", err)
	}
	if build.flusherDot, err = makeDot(slices.Concat([]Provider{dcInstance, dcReq}, dot, []Provider{dcFlush})); err != nil {
		_ = closeProviders(dot)
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
// [WithOnClose] callbacks. It is safe to call more than once. [Server] calls
// Close when retiring an instance on reload or stop; callers of
// [Config.Instance] should call Close when the instance is no longer needed if
// providers own connections or OnClose was used.
func (x *Instance) Close() error {
	if x == nil {
		return nil
	}
	err := closeProviders(x.providers)
	x.providers = nil
	err = errors.Join(err, runOnClose(x.onClose))
	x.onClose = nil
	return err
}

func closeProviders(providers []Provider) error {
	var errs []error
	// Close in reverse init order so dependents shut down before dependencies.
	for i := len(providers) - 1; i >= 0; i-- {
		if c, ok := providers[i].(Closer); ok {
			if err := c.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close %s: %w", providers[i].FieldName(), err))
			}
		}
	}
	return errors.Join(errs...)
}

// runOnClose runs callbacks in reverse registration order (destructor-like).
func runOnClose(fns []func() error) error {
	var errs []error
	for i := len(fns) - 1; i >= 0; i-- {
		if fns[i] == nil {
			continue
		}
		if err := fns[i](); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

var levelDebug2 slog.Level = slog.LevelDebug + 2

func (instance *Instance) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Add before checking cancel so waitInFlight cannot observe 0 while a
	// request is still between "not cancelled" and the increment (WaitGroup
	// would panic on Add-after-Wait; an atomic counter does not).
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
