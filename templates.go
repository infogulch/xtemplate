// caddy-xtemplate is a Caddy module that extends Go's html/template to be
// capable enough to host an entire server-side application in it.
package xtemplate

import (
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template/parse"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/fsnotify/fsnotify"
	"github.com/infogulch/pathmatcher"
)

type Templates struct {
	TemplateFS fs.FS
	ContextFS  fs.FS
	ExtraFuncs template.FuncMap
	DB         *sql.DB
	WatchPaths []string
	Config     map[string]string
	Delims     struct{ L, R string }

	log    *slog.Logger
	funcs  template.FuncMap
	router *pathmatcher.HttpMatcher[template.Template]
	stop   chan<- struct{}
	tmpl   *template.Template
}

func (t *Templates) initRouter() error {
	log := t.log.WithGroup("xtemplate-init")

	// Init funcs
	{
		funcs := make(template.FuncMap)
		for _, fm := range []template.FuncMap{t.ExtraFuncs, sprig.GenericFuncMap(), xtemplateFuncs} {
			for n, f := range fm {
				funcs[n] = f
			}
		}
		t.funcs = funcs
	}

	// Define the template instance that will accumulate all template definitions.
	templates := template.New(".").Delims(t.Delims.L, t.Delims.R).Funcs(t.funcs)

	// Find all files and send the ones that match *.html into a channel. Will check walkErr later.
	files := make(chan string)
	var walkErr error
	go func() {
		walkErr = fs.WalkDir(t.TemplateFS, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if ext := filepath.Ext(path); ext == ".html" {
				files <- path
			} else {
				log.Debug("file ignored", "path", path, "ext", ext)
			}
			return err
		})
		close(files)
	}()

	// Ingest all templates; add GET handlers for template files that don't start with '_'
	for path := range files {
		content, err := fs.ReadFile(t.TemplateFS, path)
		if err != nil {
			return fmt.Errorf("could not read template file '%s': %v", path, err)
		}
		path = filepath.Clean("/" + path)
		// parse each template file manually to have more control over its final
		// names in the template namespace.
		newtemplates, err := parse.Parse(path, string(content), t.Delims.L, t.Delims.R, t.funcs, buliltinsSkeleton)
		if err != nil {
			return fmt.Errorf("could not parse template file '%s': %v", path, err)
		}
		// add all templates
		for name, tree := range newtemplates {
			_, err = templates.AddParseTree(name, tree)
			if err != nil {
				return fmt.Errorf("could not add template '%s' from '%s': %v", name, path, err)
			}
		}
		// add the route handler template
		if !strings.HasPrefix(filepath.Base(path), "_") {
			route := "GET " + strings.TrimSuffix(path, filepath.Ext(path))
			log.Debug("adding filename route template", "route", route, "path", path)
			_, err = templates.AddParseTree(route, newtemplates[path])
			if err != nil {
				return fmt.Errorf("could not add parse tree from '%s': %v", path, err)
			}
		}
	}

	if walkErr != nil {
		return fmt.Errorf("error scanning file tree: %v", walkErr)
	}

	// Invoke all initilization templates, aka any template whose name starts with "INIT "
	for _, tmpl := range templates.Templates() {
		if strings.HasPrefix(tmpl.Name(), "INIT ") {
			var tx *sql.Tx
			var err error
			if t.DB != nil {
				tx, err = t.DB.Begin()
				if err != nil {
					return fmt.Errorf("failed to begin transaction for '%s': %v", tmpl.Name(), err)
				}
			}
			err = tmpl.Execute(io.Discard, &TemplateContext{
				tmpl:   templates,
				funcs:  t.funcs,
				fs:     t.ContextFS,
				log:    log,
				tx:     tx,
				Config: t.Config,
			})
			if err != nil {
				if tx != nil {
					tx.Rollback()
				}
				return fmt.Errorf("template initializer '%s' failed: %v", tmpl.Name(), err)
			}
			if tx != nil {
				tx.Commit()
			}
		}
	}

	// Add all routing templates to the internal router
	router := pathmatcher.NewHttpMatcher[template.Template]()
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
		log.Debug("adding route handler", "method", method, "path", path_, "template_name", tmpl.Name())
		tmpl := tmpl // create unique variable for closure
		router.AddEndpoint(method, path_, tmpl)
		count += 1
	}

	err := t.initWatcher()
	if err != nil {
		return err
	}

	// Important! Set t.router as the very last step to not confuse the watcher
	// state machine.
	t.router = router
	t.tmpl = templates
	return nil
}

func (t *Templates) initWatcher() error {
	if len(t.WatchPaths) == 0 {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	// Watch every directory under WatchPaths, recursively, as recommended by `watcher.Add` docs.
	for _, path := range t.WatchPaths {
		err = filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
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
			return fmt.Errorf("scanning for directories to watch failed: %v", err)
		}
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
	t.stop = halt
	go func() {
		delay := 200 * time.Millisecond
		var timer *time.Timer
		t.log.Info("started watching files", "directories", t.WatchPaths)
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
			t.log.Info("failed to reload templates", "error", err)
			goto begin
		}
	halt:
		watcher.Close()
		t.log.Info("closed watcher")
	}()
	return nil
}

// Cleanup discards resources held by t. Implements caddy.CleanerUpper.
func (t *Templates) Cleanup() error {
	t.router = nil
	t.funcs = nil
	if t.DB != nil {
		t.DB.Close()
		t.DB = nil
	}
	if t.stop != nil {
		t.stop <- struct{}{}
	}

	return nil
}
