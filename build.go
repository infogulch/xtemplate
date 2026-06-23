package xtemplate

// These types and methods are used while creating an instance

import (
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template/parse"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/spf13/afero"
	"github.com/tdewolff/minify/v2"
)

type builder struct {
	*Instance
	*InstanceStats
	m      *minify.M
	routes []InstanceRoute
	router *http.ServeMux
}

type InstanceStats struct {
	Routes                        int
	TemplateFiles                 int
	TemplateDefinitions           int
	TemplateInitializers          int
	StaticFiles                   int
	StaticFilesAlternateEncodings int
}

type InstanceRoute struct {
	Pattern string
	Handler http.Handler
}

type fileInfo struct {
	identityPath, hash, contentType string
	encodings                       []encodingInfo
}

type encodingInfo struct {
	encoding, path string
	size           int64
	modtime        time.Time
}

// encodingExts maps a content-encoding name to the file extension used for pre-compressed files.
var encodingExts = map[string]string{
	"gzip": ".gz",
	"zstd": ".zst",
	"br":   ".br",
}

var extensionContentTypes = map[string]string{
	".css": "text/css; charset=utf-8",
	".js":  "text/javascript; charset=utf-8",
	".csv": "text/csv",
}

func (b *builder) addStaticFileHandler(path_ string) error {
	// Open and stat the file
	fsfile, err := b.config.TemplatesFS.Open(path_)
	if err != nil {
		return fmt.Errorf("failed to open static file '%s': %w", path_, err)
	}
	defer func() { _ = fsfile.Close() }()
	seeker := fsfile.(io.ReadSeeker)
	stat, err := fsfile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file '%s': %w", path_, err)
	}
	size := stat.Size()

	var file *fileInfo
	var encoding string
	var sri string
	// Calculate the file hash. If there's a compressed file with the same
	// prefix, calculate the hash of the contents and check that they match.
	ext := filepath.Ext(path_)
	identityPath := strings.TrimSuffix(path.Clean("/"+path_), ext)
	var reader io.Reader = fsfile
	encoding = "identity"
	var exists bool
	// This relies on the identity file (e.g. "foo.css") being walked before any of
	// its compressed siblings ("foo.css.gz", "foo.css.zst", "foo.css.br"). afero.Walk
	// visits entries in lexical order, and the identity name is a prefix of each
	// compressed name, so it always sorts first. If that ordering ever changed, a
	// compressed file could be processed before its identity entry exists here and
	// would be misregistered as its own identity file.
	file, exists = b.files[identityPath]
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
		identityPath = path.Clean("/" + path_)
		file = &fileInfo{}
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
		// identity is always processed first (see the lexical-ordering note above),
		// so this is where a file's canonical metadata is established.
		file.hash = sri
		file.identityPath = identityPath
		if ctype, ok := extensionContentTypes[ext]; ok {
			file.contentType = ctype
		} else {
			content := make([]byte, 512)
			_, _ = seeker.Seek(0, io.SeekStart)
			count, err := seeker.Read(content)
			if err != nil && err != io.EOF {
				return fmt.Errorf("failed to read file to guess content type '%s': %w", path_, err)
			}
			file.contentType = http.DetectContentType(content[:count])
		}
		file.encodings = []encodingInfo{{encoding: encoding, path: path_, size: size, modtime: stat.ModTime()}}

		pattern := "GET " + identityPath
		handler := staticFileHandler(b.config.TemplatesFS, file)
		if err = catch(fmt.Sprintf("add handler to servemux '%s'", pattern), func() { b.router.HandleFunc(pattern, handler) }); err != nil {
			return err
		}
		b.StaticFiles += 1
		b.Routes += 1
		b.files[identityPath] = file
		b.routes = append(b.routes, InstanceRoute{pattern, handler})

		b.config.Logger.Debug("added static file handler", slog.String("path", identityPath), slog.String("filepath", path_), slog.String("contenttype", file.contentType), slog.Int64("size", size), slog.Time("modtime", stat.ModTime()), slog.String("hash", sri))

		for _, enc := range b.config.Precompress {
			compressedPath := path_ + encodingExts[enc]
			if _, statErr := b.config.TemplatesFS.Stat(compressedPath); statErr == nil {
				b.config.Logger.Debug("skipping precompression, file already exists", slog.String("path", compressedPath))
				continue
			}
			b.config.Logger.Debug("precompressing static file", slog.String("src", path_), slog.String("dst", compressedPath), slog.String("encoding", enc))
			if err = precompressFile(b.config.TemplatesFS, path_, enc); err != nil {
				return fmt.Errorf("failed to precompress '%s' as %s: %w", path_, enc, err)
			}
			if err = b.addStaticFileHandler(compressedPath); err != nil {
				return err
			}
		}
	} else {
		if file.hash != sri {
			return fmt.Errorf("encoded file contents did not match original file '%s': expected %s, got %s", path_, file.hash, sri)
		}
		file.encodings = append(file.encodings, encodingInfo{encoding: encoding, path: path_, size: size, modtime: stat.ModTime()})
		sort.Slice(file.encodings, func(i, j int) bool { return file.encodings[i].size < file.encodings[j].size })
		b.StaticFilesAlternateEncodings += 1
		b.config.Logger.Debug("added static file encoding", slog.String("path", identityPath), slog.String("filepath", path_), slog.String("encoding", encoding), slog.Int64("size", size), slog.Time("modtime", stat.ModTime()))
	}
	return nil
}

func catch(description string, fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("failed to %s: %v", description, r)
		}
	}()
	fn()
	return
}

var routeMatcher *regexp.Regexp = regexp.MustCompile("^(GET|POST|PUT|PATCH|DELETE|SSE) (.*)$")

func (b *builder) addTemplateHandler(path_ string) error {
	content, err := afero.ReadFile(b.config.TemplatesFS, path_)
	if err != nil {
		return fmt.Errorf("could not read template file '%s': %v", path_, err)
	}
	if b.m != nil {
		content, err = b.m.Bytes("text/html", content)
		if err != nil {
			return fmt.Errorf("could not minify template file '%s': %v", path_, err)
		}
	}
	path_ = path.Clean("/" + path_)
	// parse each template file manually to have more control over its final
	// names in the template namespace.
	newtemplates, err := parse.Parse(path_, string(content), b.config.LDelim, b.config.RDelim, b.funcs, builtinsSkeleton)
	if err != nil {
		return fmt.Errorf("could not parse template file '%s': %v", path_, err)
	}
	b.TemplateFiles += 1

	// add parsed templates, register handlers
	for name, tree := range newtemplates {
		if b.templates.Lookup(name) != nil {
			b.config.Logger.Debug("overriding named template", slog.String("name", name), slog.String("file", path_))
		}
		tmpl, err := b.templates.AddParseTree(name, tree)
		if err != nil {
			return fmt.Errorf("could not add template '%s' from '%s': %v", name, path_, err)
		}
		b.TemplateDefinitions += 1

		var pattern string
		var handler http.HandlerFunc
		if name == path_ {
			// don't register routes to hidden files
			_, file := filepath.Split(path_)
			if len(file) > 0 && file[0] == '.' {
				continue
			}
			// strip the extension from the handled path
			routePath := strings.TrimSuffix(path_, b.config.TemplateExtension)
			// files named 'index' handle requests to the directory
			base := path.Base(routePath)
			switch base {
			case "index", "index{$}":
				// index handles the directory at its canonical trailing-slash URL as an
				// exact match. ServeMux auto-redirects the slashless form (/dir -> /dir/).
				// It is NOT a subtree, so unmatched sub-paths fall through to 404; use a
				// path variable (e.g. {path...}) for catch-all behavior.
				dir := path.Clean(path.Dir(routePath))
				if dir == "/" {
					routePath = "/{$}"
				} else {
					routePath = dir + "/{$}"
				}
			default:
				routePath = path.Clean(routePath)
			}
			pattern = "GET " + routePath
			handler = bufferingTemplateHandler(b.Instance, tmpl)
		} else if matches := routeMatcher.FindStringSubmatch(name); len(matches) == 3 {
			method, path_ := matches[1], matches[2]
			if method == "SSE" {
				pattern = "GET " + path_
				handler = flushingTemplateHandler(b.Instance, tmpl)
			} else {
				pattern = method + " " + path_
				handler = bufferingTemplateHandler(b.Instance, tmpl)
			}
		} else {
			continue
		}

		if err = catch(fmt.Sprintf("add handler to servemux '%s'", pattern), func() { b.router.HandleFunc(pattern, handler) }); err != nil {
			return err
		}
		b.routes = append(b.routes, InstanceRoute{pattern, handler})
		b.Routes += 1
		b.config.Logger.Debug("added template handler", "method", "GET", "pattern", pattern, "template_path", path_)
	}
	return nil
}

// precompressFile compresses the file at srcPath to srcPath + encodingExts[encoding]
// using the given encoding ("gzip", "zstd", or "br") and encoding to extension mapping.
func precompressFile(fs afero.Fs, srcPath string, encoding string) (retErr error) {
	dstPath := srcPath + encodingExts[encoding]
	if err := fs.MkdirAll(path.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent dir for '%s': %w", dstPath, err)
	}
	in, err := fs.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, in.Close()) }()

	out, err := fs.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, out.Close()) }()

	var w io.WriteCloser
	switch encoding {
	case "gzip":
		w, err = gzip.NewWriterLevel(out, gzip.BestCompression)
		if err != nil {
			return err
		}
	case "zstd":
		w, err = zstd.NewWriter(out, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
		if err != nil {
			return err
		}
	case "br":
		w = brotli.NewWriterLevel(out, brotli.BestCompression)
	default:
		return fmt.Errorf("unsupported precompress encoding: %s", encoding)
	}

	if _, err := io.Copy(w, in); err != nil {
		return errors.Join(err, w.Close())
	}
	return w.Close()
}
