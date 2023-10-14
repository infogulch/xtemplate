// xtemplate extends Go's html/template to be capable enough to define an entire
// server-side application with just templates.
package xtemplate

import (
	"compress/gzip"
	"crypto/sha512"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template/parse"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/andybalholm/brotli"
	"github.com/infogulch/pathmatcher"
	"github.com/klauspost/compress/zstd"
)

type XTemplate struct {
	TemplateFS fs.FS
	ContextFS  fs.FS
	ExtraFuncs []template.FuncMap
	DB         *sql.DB
	Config     map[string]string
	Delims     struct{ L, R string }
	Log        *slog.Logger

	runtime *runtime
}

type runtime struct {
	templateFS fs.FS
	contextFS  fs.FS
	config     map[string]string
	funcs      template.FuncMap
	templates  *template.Template
	router     *pathmatcher.HttpMatcher[*template.Template]
	files      map[string]fileInfo
}

type fileInfo struct {
	hash, contentType string
	encodings         []encodingInfo
}

type encodingInfo struct {
	encoding, path string
	size           int64
	modtime        time.Time
}

func (t *XTemplate) Reload() error {
	log := t.Log.WithGroup("reload")

	r := &runtime{
		templateFS: t.TemplateFS,
		contextFS:  t.ContextFS,
		config:     t.Config,
		funcs:      make(template.FuncMap),
		files:      make(map[string]fileInfo),
		router:     pathmatcher.NewHttpMatcher[*template.Template](),
	}

	// Init funcs
	for _, fm := range append(t.ExtraFuncs, sprig.GenericFuncMap(), xtemplateFuncs) {
		for n, f := range fm {
			r.funcs[n] = f
		}
	}

	// Define the template instance that will accumulate all template definitions.
	r.templates = template.New(".").Delims(t.Delims.L, t.Delims.R).Funcs(r.funcs)

	// Find all files and send the ones that match *.html into a channel. Will check walkErr later.
	files := make(chan string)
	var walkErr error
	go func() {
		walkErr = fs.WalkDir(t.TemplateFS, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			files <- path
			return nil
		})
		close(files)
	}()

	// Ingest all templates; add GET handlers for template files that don't start with '_'
	for path_ := range files {

		if ext := filepath.Ext(path_); ext != ".html" {
			fsfile, err := r.templateFS.Open(path_)
			if err != nil {
				return fmt.Errorf("could not open raw file '%s': %w", path_, err)
			}
			defer fsfile.Close()
			seeker := fsfile.(io.ReadSeeker)
			stat, err := fsfile.Stat()
			if err != nil {
				return fmt.Errorf("could not stat file '%s': %w", path_, err)
			}
			size := stat.Size()

			basepath := strings.TrimSuffix(path.Clean("/"+path_), ext)
			var sri string
			var reader io.Reader = fsfile
			var encoding string = "identity"
			file, exists := r.files[basepath]
			if exists {
				switch ext {
				case ".gz":
					reader, err = gzip.NewReader(seeker)
					encoding = "gzip"
				case ".zst":
					reader, err = zstd.NewReader(seeker)
					encoding = "zstd"
				case ".br":
					reader = brotli.NewReader(seeker)
					encoding = "br"
				}
				if err != nil {
					return fmt.Errorf("could not create decompressor for file `%s`: %w", path_, err)
				}
			} else {
				basepath = path.Clean("/" + path_)
			}
			{
				hash := sha512.New384()
				_, err = io.Copy(hash, reader)
				if err != nil {
					return fmt.Errorf("could not hash file %w", err)
				}
				sri = "sha384-" + base64.StdEncoding.EncodeToString(hash.Sum(nil))
			}
			if encoding == "identity" {
				// note: identity file will always be found first because fs.WalkDir sorts files in lexical order
				file.hash = sri
				if ctype, ok := extensionContentTypes[ext]; ok {
					file.contentType = ctype
				} else {
					content := make([]byte, 512)
					seeker.Seek(0, io.SeekStart)
					count, err := seeker.Read(content)
					if err != nil && err != io.EOF {
						return fmt.Errorf("failed to read file to guess content type '%s': %w", path_, err)
					}
					file.contentType = http.DetectContentType(content[:count])
				}
				file.encodings = []encodingInfo{{encoding: encoding, path: path_, size: size, modtime: stat.ModTime()}}
				r.templates.AddParseTree("GET "+basepath, serveFileTemplate)
				r.templates.AddParseTree("HEAD "+basepath, serveFileTemplate)
				log.Debug("added new direct serve file handler", slog.String("requestpath", basepath), slog.String("filepath", path_), slog.String("contenttype", file.contentType), slog.String("hash", sri), slog.Int64("size", size))
			} else {
				if file.hash != sri {
					return fmt.Errorf("encoded file contents did not match original file '%s': expected %s, got %s", path_, file.hash, sri)
				}
				file.encodings = append(file.encodings, encodingInfo{encoding: encoding, path: path_, size: size, modtime: stat.ModTime()})
				log.Debug("added new encoding to serve file", slog.String("requestpath", basepath), slog.String("filepath", path_), slog.String("encoding", encoding), slog.Int64("size", size), slog.Time("modtime", stat.ModTime()))
			}
			r.files[basepath] = file
			continue
		}

		content, err := fs.ReadFile(t.TemplateFS, path_)
		if err != nil {
			return fmt.Errorf("could not read template file '%s': %v", path_, err)
		}
		path_ = path.Clean("/" + path_)
		// parse each template file manually to have more control over its final
		// names in the template namespace.
		newtemplates, err := parse.Parse(path_, string(content), t.Delims.L, t.Delims.R, r.funcs, buliltinsSkeleton)
		if err != nil {
			return fmt.Errorf("could not parse template file '%s': %v", path_, err)
		}
		// add all templates
		for name, tree := range newtemplates {
			_, err = r.templates.AddParseTree(name, tree)
			if err != nil {
				return fmt.Errorf("could not add template '%s' from '%s': %v", name, path_, err)
			}
		}
		// add the route handler template
		if !strings.HasPrefix(filepath.Base(path_), "_") {
			routePath := strings.TrimSuffix(path_, filepath.Ext(path_))
			if path.Base(routePath) == "index" {
				routePath = path.Dir(routePath)
			}
			route := "GET " + routePath
			log.Debug("adding filename route template", "route", route, "routePath", routePath, "path", path_)
			_, err = r.templates.AddParseTree(route, newtemplates[path_])
			if err != nil {
				return fmt.Errorf("could not add parse tree from '%s': %v", path_, err)
			}
		}
	}

	if walkErr != nil {
		return fmt.Errorf("error scanning file tree: %v", walkErr)
	}

	// Invoke all initilization templates, aka any template whose name starts with "INIT "
	for _, tmpl := range r.templates.Templates() {
		if strings.HasPrefix(tmpl.Name(), "INIT ") {
			var tx *sql.Tx
			var err error
			if t.DB != nil {
				tx, err = t.DB.Begin()
				if err != nil {
					return fmt.Errorf("failed to begin transaction for '%s': %w", tmpl.Name(), err)
				}
			}
			err = tmpl.Execute(io.Discard, &TemplateContext{
				runtime: r,
				log:     log,
				tx:      tx,
			})
			if err != nil {
				if tx != nil {
					txerr := tx.Rollback()
					if txerr != nil {
						err = errors.Join(err, txerr)
					}
				}
				return fmt.Errorf("template initializer '%s' failed: %w", tmpl.Name(), err)
			}
			if tx != nil {
				err = tx.Commit()
				if err != nil {
					return fmt.Errorf("template initializer commit failed: %w", err)
				}
			}
		}
	}

	// Add all routing templates to the internal router
	matcher, _ := regexp.Compile("^(GET|POST|PUT|PATCH|DELETE) (.*)$")
	count := 0
	for _, tmpl := range r.templates.Templates() {
		matches := matcher.FindStringSubmatch(tmpl.Name())
		if len(matches) != 3 {
			continue
		}
		method, path_ := matches[1], matches[2]
		log.Debug("adding route handler", "method", method, "path", path_, "template_name", tmpl.Name())
		tmpl := tmpl // create unique variable for closure
		r.router.Add(method, path_, tmpl)
		count += 1
	}

	// Set runtime in one pointer assignment, avoiding race conditions where the
	// inner fields don't match.
	t.runtime = r
	return nil
}

var serveFileTemplate *parse.Tree

func init() {
	serveFiles, _ := parse.Parse("servefile", "{{.ServeFile}}", "{{", "}}")
	serveFileTemplate = serveFiles["servefile"]
}

var extensionContentTypes map[string]string = map[string]string{
	".css": "text/css; charset=utf-8",
	".js":  "text/javascript; charset=utf-8",
	".csv": "text/csv",
}
