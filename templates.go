// caddy-xtemplates is a Caddy module that extends Go's html/template to be
// capable enough to host an entire server-side application in it.
package xtemplates

import (
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template/parse"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/fsnotify/fsnotify"
	sprig "github.com/go-task/slim-sprig"
	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
	"golang.org/x/exp/constraints"
	"golang.org/x/exp/slices"
)

type Templates struct {
	// The filesystem from which to load template files. May be "native"
	// (default), or the caddy module ID of a module that implements the
	// CustomFSProvider interface
	FSModule caddy.ModuleID `json:"fs_module,omitempty"`

	// The root path from which to load template files within the selected
	// filesystem (the native filesystem by default). Default is the current
	// working directory.
	Root string `json:"root,omitempty"`

	// The template action delimiters. If set, must be precisely two elements:
	// the opening and closing delimiters. Default: `["{{", "}}"]`
	Delimiters []string `json:"delimiters,omitempty"`

	// A list of caddy module IDs from which to load template FuncMaps, by
	FuncModules []caddy.ModuleID `json:"func_modules,omitempty"`

	// The database driver and connection string. If set, must be precicely two
	// elements: the driver name and the connection string.
	Database struct {
		Driver  string `json:"driver,omitempty"`
		Connstr string `json:"connstr,omitempty"`
	} `json:"database,omitempty"`

	ctx         caddy.Context
	fs          fs.FS
	customFuncs template.FuncMap
	router      *httprouter.Router
	db          *sql.DB
	stopWatcher chan<- struct{}
}

// Validate ensures t has a valid configuration. Implements caddy.Validator.
func (t *Templates) Validate() error {
	if len(t.Delimiters) != 0 && len(t.Delimiters) != 2 {
		return fmt.Errorf("delimiters must consist of exactly two elements: opening and closing")
	}
	if t.Database.Driver != "" && slices.Index(sql.Drivers(), t.Database.Driver) == -1 {
		return fmt.Errorf("database driver '%s' does not exist", t.Database.Driver)
	}
	return nil
}

// Provision provisions t. Implements caddy.Provisioner.
func (t *Templates) Provision(ctx caddy.Context) error {
	var err error

	t.ctx = ctx

	err = t.initFS()
	if err != nil {
		return err
	}

	err = t.initFuncs()
	if err != nil {
		return err
	}

	err = t.initDB()
	if err != nil {
		return err
	}

	err = t.initRouter()
	if err != nil {
		return err
	}

	return nil
}

func (t *Templates) initFS() error {
	var root string
	if len(t.Root) > 0 {
		root = t.Root
	} else {
		root = "."
	}

	if t.FSModule == "" || t.FSModule == "native" {
		t.fs = os.DirFS(root)
	} else {
		modInfo, err := caddy.GetModule(string(t.FSModule))
		if err != nil {
			return fmt.Errorf("module '%s' not found", t.FSModule)
		}
		mod := modInfo.New()
		fsp, ok := mod.(CustomFSProvider)
		if !ok {
			return fmt.Errorf("module %s does not implement TemplatesFSProvider", t.FSModule)
		}
		t.fs = fsp.CustomTemplateFS()
	}

	if st, err := fs.Stat(t.fs, "."); err != nil || !st.IsDir() {
		return fmt.Errorf("root file path does not exist in filesystem")
	}

	return nil
}

func (t *Templates) initFuncs() error {
	funcs := make(template.FuncMap)
	merge := func(m template.FuncMap) {
		for name, fn := range m {
			funcs[name] = fn
		}
	}

	for _, modid := range t.FuncModules {
		modInfo, err := caddy.GetModule(string(modid))
		if err != nil {
			return fmt.Errorf("module '%s' does not exist", modid)
		}
		mod := modInfo.New()
		fnMod, ok := mod.(CustomFunctionsProvider)
		if !ok {
			return fmt.Errorf("module %q does not satisfy the CustomFunctions interface", modid)
		}
		merge(fnMod.CustomTemplateFunctions())
	}

	merge(sprig.HtmlFuncMap())

	// add our own library
	merge(template.FuncMap{
		"stripHTML":        funcStripHTML,
		"markdown":         funcMarkdown,
		"splitFrontMatter": funcSplitFrontMatter,
		"httpError":        funcHTTPError,
		"humanize":         funcHumanize,
		"trustHtml":        funcTrustHtml,
		"trustAttr":        funcTrustAttr,
		"trustJS":          funcTrustJS,
		"trustJSStr":       funcTrustJSStr,
		"trustSrcSet":      funcTrustSrcSet,
		"uuid":             funcUuid,
		"idx":              funcIdx,
		"ksuid":            funcKsuid,
	})

	t.customFuncs = funcs
	return nil
}

func (t *Templates) initRouter() error {
	logger := t.ctx.Logger().Named("provision.router")

	dl, dr := "{{", "}}"
	if len(t.Delimiters) != 0 {
		dl, dr = t.Delimiters[0], t.Delimiters[1]
	}

	// Define the template instance that will accumulate all template definitions.
	templates := template.New(".").Delims(dl, dr).Funcs(t.customFuncs)

	// Find all files and send the ones that match *.html into a channel. Will check walkErr later.
	files := make(chan string)
	var walkErr error
	go func() {
		walkErr = fs.WalkDir(t.fs, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if ext := filepath.Ext(path); ext == ".html" {
				files <- path
			} else {
				logger.Debug("file ignored", zap.String("path", path), zap.String("ext", ext))
			}
			return err
		})
		close(files)
	}()

	// Ingest all templates; add GET handlers for template files that don't start with '_'
	for path := range files {
		content, err := fs.ReadFile(t.fs, path)
		if err != nil {
			return err
		}
		path = filepath.Clean("/" + path)
		logger.Debug("found template file", zap.Any("path", path), zap.String("startswith", string(content[:min(len(content), 20)])))
		// parse each template file manually to have more control over its final
		// names in the template namespace.
		newtemplates, err := parse.Parse(path, string(content), dl, dr, t.customFuncs, buliltinsSkeleton)
		if err != nil {
			return err
		}
		// add all templates
		for name, tree := range newtemplates {
			logger.Debug("adding defined template", zap.String("name", name), zap.String("path", path))
			_, err = templates.AddParseTree(name, tree)
			if err != nil {
				return err
			}
		}
		// add the route handler template
		if !strings.HasPrefix(filepath.Base(path), "_") {
			route := "GET " + strings.TrimSuffix(path, filepath.Ext(path))
			logger.Debug("adding filename route template", zap.String("route", route), zap.String("path", path))
			_, err = templates.AddParseTree(route, newtemplates[path])
			if err != nil {
				return err
			}
		}
	}

	if walkErr != nil {
		return walkErr
	}

	// Invoke all initilization templates, aka any template whose name starts with "INIT "
	for _, tmpl := range templates.Templates() {
		if strings.HasPrefix(tmpl.Name(), "INIT ") {
			var tx *sql.Tx
			var err error
			if t.db != nil {
				tx, err = t.db.Begin()
				if err != nil {
					// logger.Warn("failed begin database transaction", zap.Error(err))
					return caddyhttp.Error(http.StatusInternalServerError, err)
				}
			}
			err = tmpl.Execute(io.Discard, &TemplateContext{
				fs:  t.fs,
				tx:  tx,
				log: logger,
			})
			if err != nil {
				tx.Rollback()
				return err
			}
			tx.Commit()
		}
	}

	// Add all routing templates to the internal router
	router := httprouter.New()
	matcher, _ := regexp.Compile("^(GET|POST|PUT|PATCH|DELETE) (.*)$")
	count := 0
	for _, tmpl := range templates.Templates() {
		matches := matcher.FindStringSubmatch(tmpl.Name())
		if len(matches) != 3 {
			continue
		}
		method, path_ := matches[1], matches[2]
		if path.Base(path_) == "index" {
			path_ = path.Dir(path_)
		}
		logger.Debug("adding route handler", zap.String("method", method), zap.String("path", path_), zap.Any("template_name", tmpl.Name()))
		tmpl := tmpl // create unique variable for closure
		router.Handle(method, path_, func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
			*r.Context().Value("🙈").(**template.Template) = tmpl
		})
		count += 1
	}

	err := t.initWatcher()
	if err != nil {
		return err
	}

	// Important! Set t.router as the very last step to not confuse the watcher
	// state machine.
	t.router = router
	return nil
}

func (t *Templates) initDB() (err error) {
	if t.Database.Driver == "" {
		return nil
	}

	t.db, err = sql.Open(t.Database.Driver, t.Database.Connstr)
	return
}

func (t *Templates) initWatcher() error {
	// Don't watch for changes when using a custom filesystem.
	if t.FSModule != "" && t.FSModule != "native" {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	// Watch every directory under t.Root, recursively, as recommended by `watcher.Add` docs.
	err = filepath.WalkDir(t.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			err = watcher.Add(path)
		}
		return err
	})
	if err != nil {
		watcher.Close()
		return err
	}

	// The watcher state machine waits for change events from the filesystem and
	// tries to reload.
	//
	// After the first change event arrives, wait for further events until 200ms
	// passes with no events. This 'debounce' check tries to avoid a burst of
	// reloads if multiple files are changed in quick succession (e.g. editor
	// save all, or vcs checkout).
	//
	// After waiting, try to reinitialize the router and load all templates. If
	// it fails then go back to waiting again. If it succeeds then the new
	// router is already in effect and a new watcher has been created, so close
	// this one. It's easier to create a new watcher from scratch than trying to
	// interpret events to sync the watcher with the live directory structure.
	halt := make(chan struct{})
	t.stopWatcher = halt
	go func() {
		delay := 200 * time.Millisecond
		var timer *time.Timer
		t.ctx.Logger().Info("started watching files", zap.String("directory", t.Root))
	begin:
		select {
		case <-watcher.Events:
		case <-halt:
			goto halt
		}
		timer = time.NewTimer(delay)
	debounce:
		select {
		case <-watcher.Events:
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(delay)
			goto debounce
		case <-halt:
			goto halt
		case <-timer.C:
			// only fall through if the timer expires first
		}
		if err := t.initRouter(); err != nil {
			t.ctx.Logger().Info("failed to reload templates", zap.Error(err))
			goto begin
		}
	halt:
		watcher.Close()
		t.ctx.Logger().Info("closed watcher")
	}()
	return nil
}

// Cleanup discards resources held by t. Implements caddy.CleanerUpper.
func (t *Templates) Cleanup() error {
	t.router = nil
	t.fs = nil
	t.customFuncs = nil
	if t.db != nil {
		t.db.Close()
		t.db = nil
	}
	if t.stopWatcher != nil {
		t.stopWatcher <- struct{}{}
	}

	return nil
}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

// Interface guards
var (
	_ caddy.Provisioner  = (*Templates)(nil)
	_ caddy.Validator    = (*Templates)(nil)
	_ caddy.CleanerUpper = (*Templates)(nil)

	_ caddyhttp.MiddlewareHandler = (*Templates)(nil)
)
