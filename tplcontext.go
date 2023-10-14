package xtemplate

import (
	"bytes"
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/infogulch/pathmatcher"
)

// TemplateContext is the TemplateContext with which HTTP templates are executed.
type TemplateContext struct {
	Req     *http.Request
	Params  pathmatcher.Params
	Headers WrappedHeader

	status     int
	runtime    *runtime
	tx         *sql.Tx
	log        *slog.Logger
	queryTimes []time.Duration
}

func (c *TemplateContext) Config(key string) string {
	return c.runtime.config[key]
}

func (c *TemplateContext) Status(status int) string {
	c.status = status
	return ""
}

// Cookie gets the value of a cookie with name name.
func (c *TemplateContext) Cookie(name string) string {
	cookies := c.Req.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

// RemoteIP gets the IP address of the client making the request.
func (c *TemplateContext) RemoteIP() string {
	ip, _, err := net.SplitHostPort(c.Req.RemoteAddr)
	if err != nil {
		return c.Req.RemoteAddr
	}
	return ip
}

// Host returns the hostname portion of the Host header
// from the HTTP request.
func (c *TemplateContext) Host() (string, error) {
	host, _, err := net.SplitHostPort(c.Req.Host)
	if err != nil {
		if !strings.Contains(c.Req.Host, ":") {
			// common with sites served on the default port 80
			return c.Req.Host, nil
		}
		return "", err
	}
	return host, nil
}

// ReadFile returns the contents of a filename relative to the site root.
// Note that included files are NOT escaped, so you should only include
// trusted files. If it is not trusted, be sure to use escaping functions
// in your template.
func (c *TemplateContext) ReadFile(filename string) (string, error) {
	if c.runtime.contextFS == nil {
		return "", fmt.Errorf("context file system is not configured")
	}
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	filename = path.Clean(filename)
	file, err := c.runtime.contextFS.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = io.Copy(buf, file)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// StatFile returns Stat of a filename
func (c *TemplateContext) StatFile(filename string) (fs.FileInfo, error) {
	if c.runtime.contextFS == nil {
		return nil, fmt.Errorf("context file system is not configured")
	}
	filename = path.Clean(filename)
	file, err := c.runtime.contextFS.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return file.Stat()
}

// ListFiles reads and returns a slice of names from the given
// directory relative to the root of c.
func (c *TemplateContext) ListFiles(name string) ([]string, error) {
	if c.runtime.contextFS == nil {
		return nil, fmt.Errorf("context file system is not configured")
	}
	entries, err := fs.ReadDir(c.runtime.contextFS, path.Clean(name))
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, dirEntry := range entries {
		names = append(names, dirEntry.Name())
	}

	return names, nil
}

// funcFileExists returns true if filename can be opened successfully.
func (c *TemplateContext) FileExists(filename string) (bool, error) {
	if c.runtime.contextFS == nil {
		return false, fmt.Errorf("context file system is not configured")
	}
	file, err := c.runtime.contextFS.Open(filename)
	if err == nil {
		file.Close()
		return true, nil
	}
	return false, nil
}

func (c *TemplateContext) SRI(path string) (string, error) {
	fileinfo, ok := c.runtime.files[path]
	if !ok {
		return "", fmt.Errorf("file does not exist: '%s'", path)
	}
	return fileinfo.hash, nil
}

func (c *TemplateContext) ServeFile() (string, error) {
	return "", NewHandlerError("ServeFile", func(w http.ResponseWriter, r *http.Request) {
		path := path.Clean(r.URL.Path)
		fileinfo, ok := c.runtime.files[path]
		if !ok {
			c.log.Debug("tried to serve a file that doesn't exist", slog.String("path", path), slog.String("urlpath", r.URL.Path))
			http.NotFound(w, r)
			return
		}
		if queryhash := r.URL.Query().Get("hash"); queryhash != "" && queryhash != fileinfo.hash {
			c.log.Debug("request for file with wrong hash query parameter", slog.String("expected", fileinfo.hash), slog.String("queryhash", queryhash))
			http.NotFound(w, r)
			return
		}
		// TODO actually match Accept-Encoding with fileinfo.encodings instead of just picking one arbitrarily
		encoding := fileinfo.encodings[len(fileinfo.encodings)-1]
		c.log.Debug("serving file request", slog.String("path", path), slog.String("encoding", encoding.encoding), slog.String("contenttype", fileinfo.contentType))
		file, err := c.runtime.templateFS.Open(encoding.path)
		if err != nil {
			c.log.Debug("failed to open file", slog.Any("error", err), slog.String("encoding.path", encoding.path), slog.String("requestpath", r.URL.Path))
			http.Error(w, "internal server error", 500)
			return
		}
		defer file.Close()

		w.Header().Add("Etag", `"`+fileinfo.hash+`"`)
		w.Header().Add("Content-Type", fileinfo.contentType)
		w.Header().Add("Content-Encoding", encoding.encoding)
		// w.Header().Add("Access-Control-Allow-Origin", "*") // ???
		if r.URL.Query().Get("hash") != "" {
			// cache aggressively if the request is disambiguated by a valid hash
			w.Header().Set("Cache-Control", "public, max-age=31536000")
		}
		http.ServeContent(w, r, encoding.path, encoding.modtime, file.(io.ReadSeeker))
	})
}

func (c *TemplateContext) Exec(query string, params ...any) (result sql.Result, err error) {
	if c.tx == nil {
		return nil, fmt.Errorf("database is not configured")
	}
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		c.queryTimes = append(c.queryTimes, duration)
		c.log.Debug("Exec", "query", query, "params", params, "error", err)
	}()

	return c.tx.Exec(query, params...)
}

func (c *TemplateContext) QueryRows(query string, params ...any) (rows []map[string]any, err error) {
	if c.tx == nil {
		return nil, fmt.Errorf("database is not configured")
	}
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		c.queryTimes = append(c.queryTimes, duration)
		c.log.Debug("QueryRows", "query", query, "params", params, "error", err)
	}()

	result, err := c.tx.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer result.Close()

	var columns []string

	// prepare scan output array
	columns, err = result.Columns()
	if err != nil {
		return nil, err
	}
	n := len(columns)
	out := make([]any, n)
	for i := range columns {
		out[i] = new(any)
	}

	for result.Next() {
		err = result.Scan(out...)
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, n)
		for i, c := range columns {
			row[c] = *out[i].(*any)
		}
		rows = append(rows, row)
	}
	return rows, result.Err()
}

func (c *TemplateContext) QueryRow(query string, params ...any) (map[string]any, error) {
	rows, err := c.QueryRows(query, params...)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("query returned %d rows, expected exactly 1 row", len(rows))
	}
	return rows[0], nil
}

func (c *TemplateContext) QueryVal(query string, params ...any) (any, error) {
	row, err := c.QueryRow(query, params...)
	if err != nil {
		return nil, err
	}
	if len(row) != 1 {
		return nil, fmt.Errorf("query returned %d columns, expected 1", len(row))
	}
	for _, v := range row {
		return v, nil
	}
	panic("impossible condition")
}

func (c *TemplateContext) QueryStats() struct {
	Count         int
	TotalDuration time.Duration
} {
	var sum time.Duration
	for _, v := range c.queryTimes {
		sum += v
	}
	return struct {
		Count         int
		TotalDuration time.Duration
	}{
		Count:         len(c.queryTimes),
		TotalDuration: sum,
	}
}

func (c *TemplateContext) Template(name string, context any) (string, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	t := c.runtime.templates.Lookup(name)
	if t == nil {
		return "", fmt.Errorf("template name does not exist: '%s'", name)
	}
	if err := t.Execute(buf, context); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (c *TemplateContext) Funcs() template.FuncMap {
	return c.runtime.funcs
}

type TemplateContextVars struct {
	*TemplateContext
	Vars map[string]any
}

func (c *TemplateContext) WithVars(vars map[string]any) TemplateContextVars {
	return TemplateContextVars{
		TemplateContext: c,
		Vars:            vars,
	}
}

// WrappedHeader wraps niladic functions so that they
// can be used in templates. (Template functions must
// return a value.)
type WrappedHeader struct{ http.Header }

// Add adds a header field value, appending val to
// existing values for that field. It returns an
// empty string.
func (h WrappedHeader) Add(field, val string) string {
	h.Header.Add(field, val)
	return ""
}

// Set sets a header field value, overwriting any
// other values for that field. It returns an
// empty string.
func (h WrappedHeader) Set(field, val string) string {
	h.Header.Set(field, val)
	return ""
}

// Del deletes a header field. It returns an empty string.
func (h WrappedHeader) Del(field string) string {
	h.Header.Del(field)
	return ""
}

var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}
