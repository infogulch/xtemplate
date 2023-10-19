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
	"sort"
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
	delims     struct{ L, R string }
	templateFS fs.FS
	contextFS  fs.FS
	config     map[string]string
	funcs      template.FuncMap
	db         *sql.DB
	templates  *template.Template
	router     *pathmatcher.HttpMatcher[http.Handler]
	files      map[string]fileInfo
	log        *slog.Logger
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

func (t *XTemplate) Build() (http.Handler, error) {
	log := t.Log.WithGroup("reload")

	r := &runtime{
		templateFS: t.TemplateFS,
		contextFS:  t.ContextFS,
		config:     t.Config,
		delims:     t.Delims,
		db:         t.DB,
		funcs:      make(template.FuncMap),
		files:      make(map[string]fileInfo),
		router:     pathmatcher.NewHttpMatcher[http.Handler](),
		log:        t.Log,
	}

	// Init funcs
	for _, fm := range append(t.ExtraFuncs, sprig.GenericFuncMap(), xtemplateFuncs) {
		for n, f := range fm {
			r.funcs[n] = f
		}
	}

	// Define the template instance that will accumulate all template definitions.
	r.templates = template.New(".").Delims(r.delims.L, r.delims.R).Funcs(r.funcs)

	// scan all files from the templatefs root
	if err := fs.WalkDir(r.templateFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if ext := filepath.Ext(path); ext != ".html" {
			err = r.handleStaticFile(path, ext, log)
		} else {
			err = r.handleTemplateFile(path, ext, log)
		}
		if err != nil {
			log.Debug("error configuring file handler", "error", err)
		}
		return err
	}); err != nil {
		return nil, fmt.Errorf("error scanning files: %v", err)
	}

	// Invoke all initilization templates, aka any template whose name starts with "INIT "
	for _, tmpl := range r.templates.Templates() {
		if strings.HasPrefix(tmpl.Name(), "INIT ") {
			var tx *sql.Tx
			var err error
			if t.DB != nil {
				tx, err = t.DB.Begin()
				if err != nil {
					return nil, fmt.Errorf("failed to begin transaction for '%s': %w", tmpl.Name(), err)
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
				return nil, fmt.Errorf("template initializer '%s' failed: %w", tmpl.Name(), err)
			}
			if tx != nil {
				err = tx.Commit()
				if err != nil {
					return nil, fmt.Errorf("template initializer commit failed: %w", err)
				}
			}
		}
	}

	return r, nil
}

func (r *runtime) handleStaticFile(path_, ext string, log *slog.Logger) error {
	fsfile, err := r.templateFS.Open(path_)
	if err != nil {
		return fmt.Errorf("failed to open static file '%s': %w", path_, err)
	}
	defer fsfile.Close()
	seeker := fsfile.(io.ReadSeeker)
	stat, err := fsfile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file '%s': %w", path_, err)
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
			return fmt.Errorf("failed to create decompressor for file `%s`: %w", path_, err)
		}
	} else {
		basepath = path.Clean("/" + path_)
	}
	{
		hash := sha512.New384()
		_, err = io.Copy(hash, reader)
		if err != nil {
			return fmt.Errorf("failed to hash file %w", err)
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
		r.router.Add("GET", basepath, serveFileHandler)
		r.router.Add("HEAD", basepath, serveFileHandler)
		log.Debug("added static file handler", slog.String("path", basepath), slog.String("filepath", path_), slog.String("contenttype", file.contentType), slog.Int64("size", size), slog.Time("modtime", stat.ModTime()), slog.String("hash", sri))
	} else {
		if file.hash != sri {
			return fmt.Errorf("encoded file contents did not match original file '%s': expected %s, got %s", path_, file.hash, sri)
		}
		file.encodings = append(file.encodings, encodingInfo{encoding: encoding, path: path_, size: size, modtime: stat.ModTime()})
		sort.Slice(file.encodings, func(i, j int) bool { return file.encodings[i].size < file.encodings[j].size })
		log.Debug("added static file encoding", slog.String("path", basepath), slog.String("filepath", path_), slog.String("encoding", encoding), slog.Int64("size", size), slog.Time("modtime", stat.ModTime()))
	}
	r.files[basepath] = file
	return nil
}

var routeMatcher *regexp.Regexp = regexp.MustCompile("^(GET|POST|PUT|PATCH|DELETE) (.*)$")

func (r *runtime) handleTemplateFile(path_, ext string, log *slog.Logger) error {
	content, err := fs.ReadFile(r.templateFS, path_)
	if err != nil {
		return fmt.Errorf("could not read template file '%s': %v", path_, err)
	}
	path_ = path.Clean("/" + path_)
	// parse each template file manually to have more control over its final
	// names in the template namespace.
	newtemplates, err := parse.Parse(path_, string(content), r.delims.L, r.delims.R, r.funcs, buliltinsSkeleton)
	if err != nil {
		return fmt.Errorf("could not parse template file '%s': %v", path_, err)
	}
	// add all templates
	for name, tree := range newtemplates {
		tmpl, err := r.templates.AddParseTree(name, tree)
		if err != nil {
			return fmt.Errorf("could not add template '%s' from '%s': %v", name, path_, err)
		}
		if name == path_ && !strings.HasPrefix(filepath.Base(path_), "_") {
			routePath := strings.TrimSuffix(path_, filepath.Ext(path_))
			if path.Base(routePath) == "index" {
				routePath = path.Dir(routePath)
			}
			r.router.Add("GET", routePath, serveTemplateHandler(tmpl))
			log.Debug("added path template handler", "method", "GET", "path", routePath, "template_path", path_)
		} else if matches := routeMatcher.FindStringSubmatch(name); len(matches) == 3 {
			method, path_ := matches[1], matches[2]
			r.router.Add(method, path_, serveTemplateHandler(tmpl))
			log.Debug("added named template handler", "method", method, "path", path_, "template_name", name, "template_path", path_)
		}
	}
	return nil
}

var extensionContentTypes map[string]string = map[string]string{
	".css": "text/css; charset=utf-8",
	".js":  "text/javascript; charset=utf-8",
	".csv": "text/csv",
}
