package xtemplates

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
)

// TemplateContext is the TemplateContext with which HTTP templates are executed.
type TemplateContext struct {
	Req        *http.Request
	Params     httprouter.Params
	RespHeader WrappedHeader
	RespStatus func(int) string
	Next       caddyhttp.Handler

	fs  fs.FS
	tx  *sql.Tx
	log *zap.Logger
}

// OriginalReq returns the original, unmodified, un-rewritten request as
// it originally came in over the wire.
func (c *TemplateContext) OriginalReq() http.Request {
	or, _ := c.Req.Context().Value(caddyhttp.OriginalRequestCtxKey).(http.Request)
	return or
}

func (c *TemplateContext) Placeholder(name string) string {
	repl := c.Req.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)
	value, _ := repl.GetString(name)
	return value
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
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	filename = path.Clean(filename)
	file, err := c.fs.Open(filename)
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
	filename = path.Clean(filename)
	file, err := c.fs.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return file.Stat()
}

// ListFiles reads and returns a slice of names from the given
// directory relative to the root of c.
func (c *TemplateContext) ListFiles(name string) ([]string, error) {
	entries, err := fs.ReadDir(c.fs, path.Clean(name))
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
	if c.fs == nil {
		return false, fmt.Errorf("root file system not specified")
	}
	file, err := c.fs.Open(filename)
	if err == nil {
		file.Close()
		return true, nil
	}
	return false, nil
}

func (c *TemplateContext) Exec(query string, params ...any) (sql.Result, error) {
	if c.tx == nil {
		return nil, fmt.Errorf("database is not configured")
	}
	return c.tx.Exec(query, params...)
}

func (c *TemplateContext) QueryRows(query string, params ...any) ([]map[string]any, error) {
	if c.tx == nil {
		return nil, fmt.Errorf("database is not configured")
	}
	result, err := c.tx.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer result.Close()

	var rows []map[string]any
	columns, err := result.Columns()
	if err != nil {
		return nil, err
	}
	n := len(columns)
	arr := make([]any, n)
	for i, _ := range columns {
		arr[i] = new(any)
	}
	for result.Next() {
		err = result.Scan(arr...)
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, n)
		for i, c := range columns {
			row[c] = *arr[i].(*any)
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
