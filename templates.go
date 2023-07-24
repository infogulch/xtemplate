// Copyright 2015 Matthew Holt and The Caddy Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Templates is a middleware which executes response bodies as Go templates.
// The syntax is documented in the Go standard library's
// [text/template package](https://golang.org/pkg/text/template/).
//
// ⚠️ Template functions/actions are still experimental, so they are subject to change.
//
// Custom template functions can be registered by creating a plugin module under the `http.handlers.templates.functions.*` namespace that implements the `CustomFunctions` interface.
//
// [All Sprig functions](https://masterminds.github.io/sprig/) are supported.
//
// In addition to the standard functions and the Sprig library, Caddy adds
// extra functions and data that are available to a template:
//
// ##### `.Args`
//
// A slice of arguments passed to this page/context, for example as the result of a `include`.
//
// ```
// {{index .Args 0}} // first argument
// ```
//
// ##### `.Cookie`
//
// Gets the value of a cookie by name.
//
// ```
// {{.Cookie "cookiename"}}
// ```
//
// ##### `env`
//
// Gets an environment variable.
//
// ```
// {{env "VAR_NAME"}}
// ```
//
// ##### `placeholder`
//
// Gets an [placeholder variable](/docs/conventions#placeholders).
// The braces (`{}`) have to be omitted.
//
// ```
// {{placeholder "http.request.uri.path"}}
// {{placeholder "http.error.status_code"}}
// ```
//
// ##### `.Host`
//
// Returns the hostname portion (no port) of the Host header of the HTTP request.
//
// ```
// {{.Host}}
// ```
//
// ##### `httpInclude`
//
// Includes the contents of another file, and renders it in-place,
// by making a virtual HTTP request (also known as a sub-request).
// The URI path must exist on the same virtual server because the
// request does not use sockets; instead, the request is crafted in
// memory and the handler is invoked directly for increased efficiency.
//
// ```
// {{httpInclude "/foo/bar?q=val"}}
// ```
//
// ##### `import`
//
// Reads and returns the contents of another file, and parses it
// as a template, adding any template definitions to the template
// stack. If there are no definitions, the filepath will be the
// definition name. Any {{ define }} blocks will be accessible by
// {{ template }} or {{ block }}. Imports must happen before the
// template or block action is called. Note that the contents are
// NOT escaped, so you should only import trusted template files.
//
// **filename.html**
// ```
// {{ define "main" }}
// content
// {{ end }}
// ```
//
// **index.html**
// ```
// {{ import "/path/to/filename.html" }}
// {{ template "main" }}
// ```
//
// ##### `include`
//
// Includes the contents of another file, rendering it in-place.
// Optionally can pass key-value pairs as arguments to be accessed
// by the included file. Note that the contents are NOT escaped,
// so you should only include trusted template files.
//
// ```
// {{include "path/to/file.html"}}  // no arguments
// {{include "path/to/file.html" "arg1" 2 "value 3"}}  // with arguments
// ```
//
// ##### `readFile`
//
// Reads and returns the contents of another file, as-is.
// Note that the contents are NOT escaped, so you should
// only read trusted files.
//
// ```
// {{readFile "path/to/file.html"}}
// ```
//
// ##### `listFiles`
//
// Returns a list of the files in the given directory, which is relative to the template context's file root.
//
// ```
// {{listFiles "/mydir"}}
// ```
//
// ##### `markdown`
//
// Renders the given Markdown text as HTML and returns it. This uses the
// [Goldmark](https://github.com/yuin/goldmark) library,
// which is CommonMark compliant. It also has these extensions
// enabled: Github Flavored Markdown, Footnote, and syntax
// highlighting provided by [Chroma](https://github.com/alecthomas/chroma).
//
// ```
// {{markdown "My _markdown_ text"}}
// ```
//
// ##### `.RemoteIP`
//
// Returns the client's IP address.
//
// ```
// {{.RemoteIP}}
// ```
//
// ##### `.Req`
//
// Accesses the current HTTP request, which has various fields, including:
//
//   - `.Method` - the method
//   - `.URL` - the URL, which in turn has component fields (Scheme, Host, Path, etc.)
//   - `.Header` - the header fields
//   - `.Host` - the Host or :authority header of the request
//
// ```
// {{.Req.Header.Get "User-Agent"}}
// ```
//
// ##### `.OriginalReq`
//
// Like .Req, except it accesses the original HTTP request before rewrites or other internal modifications.
//
// ##### `.RespHeader.Add`
//
// Adds a header field to the HTTP response.
//
// ```
// {{.RespHeader.Add "Field-Name" "val"}}
// ```
//
// ##### `.RespHeader.Del`
//
// Deletes a header field on the HTTP response.
//
// ```
// {{.RespHeader.Del "Field-Name"}}
// ```
//
// ##### `.RespHeader.Set`
//
// Sets a header field on the HTTP response, replacing any existing value.
//
// ```
// {{.RespHeader.Set "Field-Name" "val"}}
// ```
//
// ##### `httpError`
//
// Returns an error with the given status code to the HTTP handler chain.
//
// ```
// {{if not (fileExists $includedFile)}}{{httpError 404}}{{end}}
// ```
//
// ##### `splitFrontMatter`
//
// Splits front matter out from the body. Front matter is metadata that appears at the very beginning of a file or string. Front matter can be in YAML, TOML, or JSON formats:
//
// **TOML** front matter starts and ends with `+++`:
//
// ```
// +++
// template = "blog"
// title = "Blog Homepage"
// sitename = "A Caddy site"
// +++
// ```
//
// **YAML** is surrounded by `---`:
//
// ```
// ---
// template: blog
// title: Blog Homepage
// sitename: A Caddy site
// ---
// ```
//
// **JSON** is simply `{` and `}`:
//
// ```
//
//	{
//		"template": "blog",
//		"title": "Blog Homepage",
//		"sitename": "A Caddy site"
//	}
//
// ```
//
// The resulting front matter will be made available like so:
//
// - `.Meta` to access the metadata fields, for example: `{{$parsed.Meta.title}}`
// - `.Body` to access the body after the front matter, for example: `{{markdown $parsed.Body}}`
//
// ##### `stripHTML`
//
// Removes HTML from a string.
//
// ```
// {{stripHTML "Shows <b>only</b> text content"}}
// ```
//
// ##### `humanize`
//
// Transforms size and time inputs to a human readable format.
// This uses the [go-humanize](https://github.com/dustin/go-humanize) library.
//
// The first argument must be a format type, and the last argument
// is the input, or the input can be piped in. The supported format
// types are:
// - **size** which turns an integer amount of bytes into a string like `2.3 MB`
// - **time** which turns a time string into a relative time string like `2 weeks ago`
//
// For the `time` format, the layout for parsing the input can be configured
// by appending a colon `:` followed by the desired time layout. You can
// find the documentation on time layouts [in Go's docs](https://pkg.go.dev/time#pkg-constants).
// The default time layout is `RFC1123Z`, i.e. `Mon, 02 Jan 2006 15:04:05 -0700`.
//
// ```
// {{humanize "size" "2048000"}}
// {{placeholder "http.response.header.Content-Length" | humanize "size"}}
// {{humanize "time" "Fri, 05 May 2022 15:04:05 +0200"}}
// {{humanize "time:2006-Jan-02" "2022-May-05"}}
// ```
package xtemplates

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template/parse"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/fsnotify/fsnotify"
	sprig "github.com/go-task/slim-sprig"
	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
)

type Templates struct {
	// The root path from which to load files. Required if template functions
	// accessing the file system are used (such as include). Default is
	// `{http.vars.root}` if set, or current working directory otherwise.
	FileRoot string `json:"file_root,omitempty"`

	// The template action delimiters. If set, must be precisely two elements:
	// the opening and closing delimiters. Default: `["{{", "}}"]`
	Delimiters []string `json:"delimiters,omitempty"`

	// The database driver and connection string. If set, must be precicely two
	// elements: the driver name and the connection string.
	Database []string `json:"database,omitempty"`

	ctx         caddy.Context
	fs          fs.FS
	customFuncs template.FuncMap
	router      *httprouter.Router
	db          *sql.DB
	stopWatcher chan<- struct{}
}

// Customfunctions is the interface for registering custom template functions.
type CustomFunctions interface {
	// CustomTemplateFunctions should return the mapping from custom function names to implementations.
	CustomTemplateFunctions() template.FuncMap
}

// Validate ensures t has a valid configuration. Implements caddy.Validator.
func (t *Templates) Validate() error {
	if len(t.Delimiters) != 0 && len(t.Delimiters) != 2 {
		return fmt.Errorf("delimiters must consist of exactly two elements: opening and closing")
	}
	if len(t.Database) != 0 && len(t.Database) != 2 {
		return fmt.Errorf("database connection must consist of exactly two elements: driver and connection string, got %d: %v", len(t.Database), t.Database)
	}
	if len(t.Database) == 2 {
		exists := false
		for _, driver := range sql.Drivers() {
			if driver == t.Database[0] {
				exists = true
				break
			}
		}
		if !exists {
			return fmt.Errorf("database driver '%s' does not exist", t.Database[0])
		}
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
	if len(t.FileRoot) > 0 {
		root = t.FileRoot
	} else {
		root = "."
	}

	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return fmt.Errorf("root file path does not exist")
	}
	t.fs = os.DirFS(root)

	return nil
}

func (t *Templates) initFuncs() error {
	funcs := make(template.FuncMap)
	merge := func(m template.FuncMap) {
		for name, fn := range m {
			funcs[name] = fn
		}
	}

	fnModInfos := caddy.GetModules("http.handlers.templates.functions")
	for _, modInfo := range fnModInfos {
		mod := modInfo.New()
		fnMod, ok := mod.(CustomFunctions)
		if !ok {
			return fmt.Errorf("module %q does not satisfy the CustomFunctions interface", modInfo.ID)
		}
		merge(fnMod.CustomTemplateFunctions())
	}

	merge(sprig.HtmlFuncMap())

	// add our own library
	merge(template.FuncMap{
		"stripHTML":        funcStripHTML,
		"markdown":         funcMarkdown,
		"splitFrontMatter": funcSplitFrontMatter,
		"env":              funcEnv,
		"httpError":        funcHTTPError,
		"humanize":         funcHumanize,
		"trustHtml":        funcTrustHtml,
		"trustAttr":        funcTrustAttr,
		"trustJS":          funcTrustJS,
		"trustJSStr":       funcTrustJSStr,
		"trustSrcSet":      funcTrustSrcSet,
		"uuid":             funcUuid,
		"idx":              funcIdx,
	})

	t.customFuncs = funcs
	return nil
}

func (t *Templates) initRouter() error {
	dl, dr := "{{", "}}"
	if len(t.Delimiters) != 0 {
		dl, dr = t.Delimiters[0], t.Delimiters[1]
	}

	templates := template.New(".").Delims(dl, dr).Funcs(t.customFuncs)

	files, err := fs.Glob(t.fs, "*.html")
	if err != nil {
		return err
	}

	logger := t.ctx.Logger().Named("provision")
	logger.Debug("found templates", zap.Int("count", len(files)))

	// Ingest all templates; add GET handlers for template files that don't start with '_'
	for _, path := range files {
		content, err := fs.ReadFile(t.fs, path)
		if err != nil {
			return err
		}
		// parse each template file manually to have more control over its final
		// names in the template namespace.
		newtemplates, err := parse.Parse(path, string(content), dl, dr, t.customFuncs, buliltinsSkeleton)
		if err != nil {
			return err
		}
		// add all templates
		for name, tree := range newtemplates {
			_, err = templates.AddParseTree(name, tree)
			if err != nil {
				return err
			}
		}
		// add the route handler template
		if !strings.HasPrefix(filepath.Base(path), "_") {
			route := "GET /" + strings.TrimSuffix(path, filepath.Ext(path))
			_, err = templates.AddParseTree(route, newtemplates[path])
			if err != nil {
				return err
			}
		}
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
		logger.Debug("adding route handler", zap.String("method", method), zap.String("path", path_), zap.Any("template", tmpl.Name()))
		tmpl := tmpl // create unique variable for closure
		router.Handle(method, path_, func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
			*r.Context().Value("🙈").(**template.Template) = tmpl
		})
		count += 1
	}

	t.router = router
	logger.Info("loaded router", zap.Int("routes", count))
	return t.initWatcher()
}

func (t *Templates) initDB() (err error) {
	if len(t.Database) == 0 {
		return nil
	}

	t.db, err = sql.Open(t.Database[0], t.Database[1])
	return
}

func (t *Templates) initWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	err = filepath.WalkDir(t.FileRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			err = watcher.Add(path)
		}
		return err
	})
	if err != nil {
		return err
	}
	halt := make(chan struct{})
	t.stopWatcher = halt
	go func() {
		delay := 200 * time.Millisecond
		var timer *time.Timer
		t.ctx.Logger().Info("started watching files", zap.String("directory", t.FileRoot))
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
		case <-timer.C:
		case <-halt:
			goto halt
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

func (t *Templates) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	logger := t.ctx.Logger().Named(r.URL.Path)
	handle, params, _ := t.router.Lookup(r.Method, r.URL.Path)
	if handle == nil {
		logger.Debug("no handler for request", zap.String("method", r.Method), zap.String("path", r.URL.Path))
		return caddyhttp.Error(http.StatusNotFound, nil)
	}
	var template *template.Template
	handle(nil, new(http.Request).WithContext(context.WithValue(context.Background(), "🙈", &template)), nil)
	logger.Debug("handling request", zap.String("method", r.Method), zap.String("path", r.URL.Path), zap.Any("params", params), zap.String("name", template.Name()))

	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	alwaysbuffer := func(_ int, _ http.Header) bool { return true }
	rec := caddyhttp.NewResponseRecorder(w, buf, alwaysbuffer)

	var tx *sql.Tx
	var err error
	if t.db != nil {
		tx, err = t.db.Begin()
		if err != nil {
			logger.Info("failed to begin database transaction", zap.Error(err))
			return caddyhttp.Error(http.StatusInternalServerError, err)
		}
	}

	r.ParseForm()
	var statusCode = 200
	context := &TemplateContext{
		Req:        r,
		Params:     params,
		RespStatus: func(c int) string { statusCode = c; return "" },
		RespHeader: WrappedHeader{w.Header()},
		Next:       next,

		fs:  t.fs,
		tx:  tx,
		log: logger,
	}

	err = template.Execute(w, context)
	if err != nil {
		var handlerErr caddyhttp.HandlerError
		if errors.As(err, &handlerErr) {
			if dberr := tx.Commit(); dberr != nil {
				logger.Info("error committing transaction", zap.Error(err))
			}
			return handlerErr
		}
		logger.Info("error executing template", zap.Error(err))
		if dberr := tx.Rollback(); dberr != nil {
			logger.Info("error rolling back transaction", zap.Error(err))
		}
		return caddyhttp.Error(http.StatusInternalServerError, err)
	} else {
		if dberr := tx.Commit(); dberr != nil {
			logger.Info("error committing transaction", zap.Error(err))
		}
	}

	rec.WriteHeader(statusCode)
	rec.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	rec.Header().Del("Accept-Ranges") // we don't know ranges for dynamically-created content
	rec.Header().Del("Last-Modified") // useless for dynamic content since it's always changing

	// we don't know a way to quickly generate etag for dynamic content,
	// and weak etags still cause browsers to rely on it even after a
	// refresh, so disable them until we find a better way to do this
	rec.Header().Del("Etag")

	return rec.WriteResponse()
}

// Interface guards
var (
	_ caddy.Provisioner  = (*Templates)(nil)
	_ caddy.Validator    = (*Templates)(nil)
	_ caddy.CleanerUpper = (*Templates)(nil)

	_ caddyhttp.MiddlewareHandler = (*Templates)(nil)
)
