package xtemplate

import (
	"compress/gzip"
	"context"
	"crypto/sha512"
	"database/sql"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"text/template/parse"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// Build creates a new xtemplate server instance, a `CancelHandler`, from an xtemplate.Config.
func Build(config *Config) (CancelHandler, error) {
	server, err := newServer(config)
	if err != nil {
		return nil, err
	}
	log := server.Logger.WithGroup("build")
	stats := &buildStats{}

	// Recursively scan and process all files in Template.FS.
	if err := fs.WalkDir(server.Template.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if ext := filepath.Ext(path); ext == server.Template.TemplateExtension {
			err = server.addTemplateHandler(path, log, stats)
		} else {
			err = server.addStaticFileHandler(path, log, stats)
		}
		return err
	}); err != nil {
		return nil, fmt.Errorf("error scanning files: %v", err)
	}

	// Invoke all initilization templates, aka any template whose name starts with "INIT ".
	for _, tmpl := range server.templates.Templates() {
		if strings.HasPrefix(tmpl.Name(), "INIT ") {
			context := &struct {
				baseContext
				fsContext
			}{
				baseContext{
					server: server,
					log:    log,
				},
				fsContext{
					fs: server.Context.FS,
				},
			}
			err := tmpl.Execute(io.Discard, context)
			if err = context.resolvePendingTx(err); err != nil {
				return nil, fmt.Errorf("template initializer '%s' failed: %w", tmpl.Name(), err)
			}
			stats.TemplateInitializers += 1
		}
	}

	log.Info("xtemplate instance initialized", slog.Any("stats", stats))
	log.Debug("xtemplate instance details", slog.Any("xtemplate", server))
	return server, nil
}

// Counter to assign a unique id to each instance of xtemplate created when
// calling Build(). This is intended to help distinguish logs from multiple
// instances in a single process.
var nextInstanceIdentity int64

// Counts of various objects created during Build.
type buildStats struct {
	Routes               int
	TemplateFiles        int
	TemplateDefinitions  int
	TemplateInitializers int
	StaticFiles          int
	StaticFileEncodings  int
}

// newServer creates an empty xserver with all data structures initalized using the provided config.
func newServer(config *Config) (*xserver, error) {
	c := &xserver{
		Config: *config,
	}
	if c.Template.FS == nil {
		if c.Template.Path == "" {
			c.Template.Path = "templates"
		}
		c.Template.FS = os.DirFS(c.Template.Path)
	}

	if c.Context.FS == nil && c.Context.Path != "" {
		c.Context.FS = os.DirFS(c.Context.Path)
	}

	if c.Database.DB == nil && c.Database.Driver != "" {
		var err error
		c.Database.DB, err = sql.Open(c.Database.Driver, c.Database.Connstr)
		if err != nil {
			return nil, fmt.Errorf("failed to open database with driver and connstr: `%s`, `%s`", c.Database.Driver, c.Database.Connstr)
		}
	}

	c.id = atomic.AddInt64(&nextInstanceIdentity, 1)

	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(c.LogLevel)}))
	}
	c.Logger = c.Logger.WithGroup("xtemplate").With(slog.Int64("instance", c.id))

	{
		c.funcs = template.FuncMap{}
		maps.Copy(c.funcs, xtemplateFuncs)
		maps.Copy(c.funcs, sprig.HtmlFuncMap())
		for _, extra := range c.FuncMaps {
			maps.Copy(c.funcs, extra)
		}
	}

	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.UserConfig = maps.Clone(c.UserConfig)
	c.files = make(map[string]fileInfo)
	c.router = http.NewServeMux()
	c.templates = template.New(".").Delims(c.Template.Delimiters.Left, c.Template.Delimiters.Right).Funcs(c.funcs)

	return c, nil
}

func (server *xserver) addHandler(pattern string, handler Handler) {
	server.router.HandleFunc(pattern, server.mainHandler(pattern, handler))
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

func (x *xserver) addStaticFileHandler(path_ string, log *slog.Logger, stats *buildStats) error {
	// Open and stat the file
	fsfile, err := x.Template.FS.Open(path_)
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

	// Calculate the file hash. If there's a compressed file with the same
	// prefix, calculate the hash of the contents and check that they match.
	ext := filepath.Ext(path_)
	basepath := strings.TrimSuffix(path.Clean("/"+path_), ext)
	var sri string
	var reader io.Reader = fsfile
	var encoding string = "identity"
	file, exists := x.files[basepath]
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
		sri = "sha384-" + base64.URLEncoding.EncodeToString(hash.Sum(nil))
	}

	// Save precalculated file size, modtime, hash, content type, and encoding
	// info to enable efficient content negotiation at request time.
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
		x.addHandler("GET "+basepath, serveFileHandler)
		stats.StaticFiles += 1
		stats.Routes += 1
		log.Debug("added static file handler", slog.String("path", basepath), slog.String("filepath", path_), slog.String("contenttype", file.contentType), slog.Int64("size", size), slog.Time("modtime", stat.ModTime()), slog.String("hash", sri))
	} else {
		if file.hash != sri {
			return fmt.Errorf("encoded file contents did not match original file '%s': expected %s, got %s", path_, file.hash, sri)
		}
		file.encodings = append(file.encodings, encodingInfo{encoding: encoding, path: path_, size: size, modtime: stat.ModTime()})
		sort.Slice(file.encodings, func(i, j int) bool { return file.encodings[i].size < file.encodings[j].size })
		stats.StaticFileEncodings += 1
		log.Debug("added static file encoding", slog.String("path", basepath), slog.String("filepath", path_), slog.String("encoding", encoding), slog.Int64("size", size), slog.Time("modtime", stat.ModTime()))
	}
	x.files[basepath] = file
	return nil
}

var routeMatcher *regexp.Regexp = regexp.MustCompile("^(GET|POST|PUT|PATCH|DELETE|SSE) (.*)$")

func (x *xserver) addTemplateHandler(path_ string, log *slog.Logger, stats *buildStats) error {
	content, err := fs.ReadFile(x.Template.FS, path_)
	if err != nil {
		return fmt.Errorf("could not read template file '%s': %v", path_, err)
	}
	path_ = path.Clean("/" + path_)
	// parse each template file manually to have more control over its final
	// names in the template namespace.
	newtemplates, err := parse.Parse(path_, string(content), x.Template.Delimiters.Left, x.Template.Delimiters.Right, x.funcs, buliltinsSkeleton)
	if err != nil {
		return fmt.Errorf("could not parse template file '%s': %v", path_, err)
	}
	stats.TemplateFiles += 1
	// add all templates
	for name, tree := range newtemplates {
		if x.templates.Lookup(name) != nil {
			log.Debug("overriding named template '%s' with definition from file: %s", name, path_)
		}
		tmpl, err := x.templates.AddParseTree(name, tree)
		if err != nil {
			return fmt.Errorf("could not add template '%s' from '%s': %v", name, path_, err)
		}
		stats.TemplateDefinitions += 1
		if name == path_ {
			// don't register routes to hidden files
			_, file := filepath.Split(path_)
			if len(file) > 0 && file[0] == '.' {
				continue
			}
			// strip the extension from the handled path
			routePath := strings.TrimSuffix(path_, x.Template.TemplateExtension)
			// files named 'index' handle requests to the directory
			if path.Base(routePath) == "index" {
				routePath = path.Dir(routePath)
			}
			if strings.HasSuffix(routePath, "/") {
				routePath += "{$}"
			}
			x.addHandler("GET "+routePath, bufferedTemplateHandler(tmpl))
			stats.Routes += 1
			log.Debug("added path template handler", "method", "GET", "path", routePath, "template_path", path_)
		} else if matches := routeMatcher.FindStringSubmatch(name); len(matches) == 3 {
			method, path_ := matches[1], matches[2]
			if method == "SSE" {
				x.addHandler("GET "+path_, sseTemplateHandler(tmpl))
			} else {
				x.addHandler(method+" "+path_, bufferedTemplateHandler(tmpl))
			}
			stats.Routes += 1
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
