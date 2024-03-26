package providers

import (
	"context"
	"database/sql"
	"encoding"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/infogulch/xtemplate"
)

func init() {
	xtemplate.RegisterDot(&DotDBProvider{})
}

func WithDB(name string, db *sql.DB, opt *sql.TxOptions) xtemplate.ConfigOverride {
	return xtemplate.WithProvider(name, &DotDBProvider{DB: db, TxOptions: opt})
}

type DotDBProvider struct {
	*sql.DB
	*sql.TxOptions
	driver, connstr string
}

func (DotDBProvider) New() xtemplate.DotProvider { return &DotDBProvider{} }
func (DotDBProvider) Name() string               { return "sql" }
func (DotDBProvider) Type() reflect.Type         { return reflect.TypeOf(&DotDB{}) }

func (d *DotDBProvider) UnmarshalText(b []byte) error {
	parts := strings.SplitN(string(b), ":", 2)
	if len(parts) < 2 {
		return fmt.Errorf("not enough parameters to configure sql dot. Requires DRIVER:CONNSTR, got: %s", string(b))
	}
	db, err := sql.Open(parts[0], parts[1])
	if err != nil {
		return fmt.Errorf("failed to open database with driver name %s: %w", parts[0], err)
	}
	err = db.Ping()
	if err != nil {
		return fmt.Errorf("failed to ping database on open: %w", err)
	}
	d.DB = db
	d.driver = parts[0]
	d.connstr = parts[1]
	return nil
}

func (d *DotDBProvider) MarshalText() ([]byte, error) {
	if d.driver == "" || d.connstr == "" {
		return nil, fmt.Errorf("cannot unmarshal because SqlDot does not have the driver and connstr")
	}
	return []byte(d.driver + ":" + d.connstr), nil
}

var _ xtemplate.CleanupDotProvider = &DotDBProvider{}
var _ encoding.TextUnmarshaler = &DotDBProvider{}
var _ encoding.TextMarshaler = &DotDBProvider{}

func (d *DotDBProvider) Value(log *slog.Logger, sctx context.Context, w http.ResponseWriter, r *http.Request) (reflect.Value, error) {
	return reflect.ValueOf(&DotDB{d.DB, log, r.Context(), d.TxOptions, nil}), nil
}

func (dp *DotDBProvider) Cleanup(v reflect.Value, err error) error {
	d := v.Interface().(*DotDB)
	if err != nil {
		return errors.Join(err, d.rollback())
	} else {
		return errors.Join(err, d.commit())
	}
}

type DotDB struct {
	db  *sql.DB
	log *slog.Logger
	ctx context.Context
	opt *sql.TxOptions
	tx  *sql.Tx
}

func (d *DotDB) makeTx() (err error) {
	if d.tx == nil {
		d.tx, err = d.db.BeginTx(d.ctx, d.opt)
	}
	return
}

func (c *DotDB) Exec(query string, params ...any) (result sql.Result, err error) {
	if err = c.makeTx(); err != nil {
		return
	}

	defer func(start time.Time) {
		c.log.Debug("Exec", slog.String("query", query), slog.Any("params", params), slog.Any("error", err), slog.Duration("queryduration", time.Since(start)))
	}(time.Now())

	return c.tx.Exec(query, params...)
}

func (c *DotDB) QueryRows(query string, params ...any) (rows []map[string]any, err error) {
	if err = c.makeTx(); err != nil {
		return
	}

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

func (c *DotDB) QueryRow(query string, params ...any) (map[string]any, error) {
	rows, err := c.QueryRows(query, params...)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("query returned %d rows, expected exactly 1 row", len(rows))
	}
	return rows[0], nil
}

func (c *DotDB) QueryVal(query string, params ...any) (any, error) {
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

func (c *DotDB) Commit() (string, error) {
	return "", c.commit()
}

func (c *DotDB) commit() error {
	if c.tx != nil {
		err := c.tx.Commit()
		c.log.Debug("commit", slog.Any("error", err))
		c.tx = nil
		return err
	}
	return nil
}

func (c *DotDB) Rollback() (string, error) {
	return "", c.rollback()
}

func (c *DotDB) rollback() error {
	if c.tx != nil {
		err := c.tx.Rollback()
		c.log.Debug("rollback", slog.Any("error", err))
		c.tx = nil
		return err
	}
	return nil
}
