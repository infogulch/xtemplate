package xtemplate

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

type baseContext struct {
	server     *xinstance
	log        *slog.Logger
	pendingTx  *sql.Tx
	requestCtx context.Context
}

func (c *baseContext) Config(key string) string {
	return c.server.UserConfig[key]
}

// ServeContent aborts execution of the template and instead responds to the request with content
func (c *baseContext) ServeContent(path_ string, modtime time.Time, content string) (string, error) {
	return "", NewHandlerError("ServeFile", func(w http.ResponseWriter, r *http.Request) {
		path_ = path.Clean(path_)

		c.log.Debug("serving content response", slog.String("path", path_))

		http.ServeContent(w, r, path_, modtime, strings.NewReader(content))
	})
}

func (c *baseContext) StaticFileHash(urlpath string) (string, error) {
	urlpath = path.Clean("/" + urlpath)
	fileinfo, ok := c.server.files[urlpath]
	if !ok {
		return "", fmt.Errorf("file does not exist: '%s'", urlpath)
	}
	return fileinfo.hash, nil
}

func (c *baseContext) Template(name string, context any) (string, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	t := c.server.templates.Lookup(name)
	if t == nil {
		return "", fmt.Errorf("template name does not exist: '%s'", name)
	}
	if err := t.Execute(buf, context); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (c *baseContext) Funcs() template.FuncMap {
	return c.server.funcs
}

func (c *baseContext) Tx() (*sqlContext, error) {
	if c.server.Database.DB == nil {
		return nil, fmt.Errorf("database is not configured")
	}
	if c.pendingTx != nil {
		if err := c.pendingTx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit pending tx: %w", err)
		}
		c.pendingTx = nil
	}
	tx, err := c.server.Database.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction")
	}
	c.pendingTx = tx
	return &sqlContext{tx: tx, log: c.log.WithGroup("tx")}, nil
}

func (c *baseContext) resolvePendingTx(err error) error {
	if c.pendingTx == nil {
		return err
	}
	if err == nil {
		err = c.pendingTx.Commit()
	} else {
		dberr := c.pendingTx.Rollback()
		if dberr != nil {
			err = errors.Join(err, dberr)
		}
	}
	c.pendingTx = nil
	return err
}

type sqlContext struct {
	tx  *sql.Tx
	log *slog.Logger
}

func (c *sqlContext) Exec(query string, params ...any) (result sql.Result, err error) {
	defer func(start time.Time) {
		c.log.Debug("Exec", slog.String("query", query), slog.Any("params", params), slog.Any("error", err), slog.Duration("queryduration", time.Since(start)))
	}(time.Now())

	return c.tx.Exec(query, params...)
}

func (c *sqlContext) QueryRows(query string, params ...any) (rows []map[string]any, err error) {
	defer func(start time.Time) {
		c.log.Debug("QueryRows", slog.String("query", query), slog.Any("params", params), slog.Any("error", err), slog.Duration("queryduration", time.Since(start)))
	}(time.Now())

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

func (c *sqlContext) QueryRow(query string, params ...any) (map[string]any, error) {
	rows, err := c.QueryRows(query, params...)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("query returned %d rows, expected exactly 1 row", len(rows))
	}
	return rows[0], nil
}

func (c *sqlContext) QueryVal(query string, params ...any) (any, error) {
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

func (c *sqlContext) Commit() (string, error) {
	err := c.tx.Commit()
	c.log.Debug("Commit", slog.Any("error", err))
	return "", err
}

func (c *sqlContext) Rollback() (string, error) {
	err := c.tx.Rollback()
	c.log.Debug("Rollback", slog.Any("error", err))
	return "", err
}

type fsContext struct {
	fs  fs.FS
	log *slog.Logger
}

// ReadFile returns the contents of a filename relative to the site root.
// Note that included files are NOT escaped, so you should only include
// trusted files. If it is not trusted, be sure to use escaping functions
// in your template.
func (c *fsContext) ReadFile(filename string) (string, error) {
	if c.fs == nil {
		return "", fmt.Errorf("context file system is not configured")
	}
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
func (c *fsContext) StatFile(filename string) (fs.FileInfo, error) {
	if c.fs == nil {
		return nil, fmt.Errorf("context file system is not configured")
	}
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
func (c *fsContext) ListFiles(name string) ([]string, error) {
	if c.fs == nil {
		return nil, fmt.Errorf("context file system is not configured")
	}
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

// FileExists returns true if filename can be opened successfully.
func (c *fsContext) FileExists(filename string) (bool, error) {
	if c.fs == nil {
		return false, fmt.Errorf("context file system is not configured")
	}
	file, err := c.fs.Open(filename)
	if err == nil {
		file.Close()
		return true, nil
	}
	return false, nil
}

// ServeFile aborts execution of the template and instead responds to the
// request with the content of the contextfs file at path_
func (c *fsContext) ServeFile(path_ string) (string, error) {
	return "", NewHandlerError("ServeFile", func(w http.ResponseWriter, r *http.Request) {
		path_ = path.Clean(path_)

		c.log.Debug("serving file response", slog.String("path", path_))

		file, err := c.fs.Open(path_)
		if err != nil {
			c.log.Debug("failed to open file", slog.Any("error", err), slog.String("path", path_))
			http.Error(w, "internal server error", 500)
			return
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			c.log.Debug("error getting stat of file", slog.Any("error", err), slog.String("path", path_))
		}

		http.ServeContent(w, r, path_, stat.ModTime(), file.(io.ReadSeeker))
	})
}

type requestContext struct {
	Req *http.Request
}

// Cookie gets the value of a cookie with name name.
func (c *requestContext) Cookie(name string) string {
	cookies := c.Req.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

// RemoteIP gets the IP address of the client making the request.
func (c *requestContext) RemoteIP() string {
	ip, _, err := net.SplitHostPort(c.Req.RemoteAddr)
	if err != nil {
		return c.Req.RemoteAddr
	}
	return ip
}

// Host returns the hostname portion of the Host header
// from the HTTP request.
func (c *requestContext) Host() (string, error) {
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

type responseContext struct {
	http.Header
	status int
}

// Add adds a header field value, appending val to
// existing values for that field. It returns an
// empty string.
func (h responseContext) AddHeader(field, val string) string {
	h.Header.Add(field, val)
	return ""
}

// Set sets a header field value, overwriting any
// other values for that field. It returns an
// empty string.
func (h responseContext) SetHeader(field, val string) string {
	h.Header.Set(field, val)
	return ""
}

// Del deletes a header field. It returns an empty string.
func (h responseContext) DelHeader(field string) string {
	h.Header.Del(field)
	return ""
}

func (h *responseContext) SetStatus(status int) string {
	h.status = status
	return ""
}

type flushContext struct {
	flusher http.Flusher
	baseContext
}

func (f flushContext) Flush() string {
	f.flusher.Flush()
	return ""
}

func (f flushContext) Repeat(max_ ...int) <-chan int {
	max := math.MaxInt64 // sorry you can only loop for 2^63-1 iterations max
	if len(max_) > 0 {
		max = max_[0]
	}
	c := make(chan int)
	go func() {
		i := 0
	loop:
		for {
			select {
			case <-f.requestCtx.Done():
				break loop
			case <-f.server.ctx.Done():
				break loop
			case c <- i:
			}
			if i >= max {
				break
			}
			i++
		}
		close(c)
	}()
	return c
}

// Sleep sleeps for ms millisecionds.
func (f flushContext) Sleep(ms int) (string, error) {
	select {
	case <-time.After(time.Duration(ms) * time.Millisecond):
	case <-f.requestCtx.Done():
		return "", ReturnError{}
	case <-f.server.ctx.Done():
		return "", ReturnError{}
	}
	return "", nil
}

// Block blocks execution until the request is canceled by the client or until
// the server closes.
func (f flushContext) Block() (string, error) {
	select {
	case <-f.requestCtx.Done():
		return "", ReturnError{}
	case <-f.server.ctx.Done():
		return "", nil
	}
}

var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}
