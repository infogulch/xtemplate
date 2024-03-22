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
	xtemplate.RegisterDot(&SqlDot{})
}

func WithDB(name string, db *sql.DB, opts ...*sql.TxOptions) xtemplate.ConfigOverride {
	var opt *sql.TxOptions
	switch len(opts) {
	case 0:
		// nothing
	case 1:
		opt = opts[0]
	default:
		panic("too many options")
	}
	return xtemplate.WithProvider(name, &SqlDot{DB: db, TxOptions: opt})
}

type SqlDot struct {
	*sql.DB
	*sql.TxOptions
	driver, connstr string
}

func (SqlDot) New() xtemplate.DotProvider { return &SqlDot{} }
func (SqlDot) Name() string               { return "sql" }
func (SqlDot) Type() reflect.Type         { return reflect.TypeOf(&sqlDot{}) }

func (d *SqlDot) UnmarshalText(b []byte) error {
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

func (d *SqlDot) MarshalText() ([]byte, error) {
	if d.driver == "" || d.connstr == "" {
		return nil, fmt.Errorf("cannot unmarshal because SqlDot does not have the driver and connstr")
	}
	return []byte(d.driver + ":" + d.connstr), nil
}

var _ xtemplate.CleanupDotProvider = &SqlDot{}
var _ encoding.TextUnmarshaler = &SqlDot{}
var _ encoding.TextMarshaler = &SqlDot{}

func (d *SqlDot) Value(log *slog.Logger, sctx context.Context, w http.ResponseWriter, r *http.Request) (reflect.Value, error) {
	return reflect.ValueOf(&sqlDot{d.DB, log, r.Context(), d.TxOptions, nil}), nil
}

func (dp *SqlDot) Cleanup(v reflect.Value, err error) error {
	d := v.Interface().(*sqlDot)
	if err != nil {
		return errors.Join(err, d.rollback())
	} else {
		return errors.Join(err, d.commit())
	}
}

type sqlDot struct {
	db  *sql.DB
	log *slog.Logger
	ctx context.Context
	opt *sql.TxOptions
	tx  *sql.Tx
}

func (d *sqlDot) makeTx() (err error) {
	if d.tx == nil {
		d.tx, err = d.db.BeginTx(d.ctx, d.opt)
	}
	return
}

func (c *sqlDot) Exec(query string, params ...any) (result sql.Result, err error) {
	if err = c.makeTx(); err != nil {
		return
	}

	defer func(start time.Time) {
		c.log.Debug("Exec", slog.String("query", query), slog.Any("params", params), slog.Any("error", err), slog.Duration("queryduration", time.Since(start)))
	}(time.Now())

	return c.tx.Exec(query, params...)
}

func (c *sqlDot) QueryRows(query string, params ...any) (rows []map[string]any, err error) {
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

func (c *sqlDot) QueryRow(query string, params ...any) (map[string]any, error) {
	rows, err := c.QueryRows(query, params...)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("query returned %d rows, expected exactly 1 row", len(rows))
	}
	return rows[0], nil
}

func (c *sqlDot) QueryVal(query string, params ...any) (any, error) {
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

func (c *sqlDot) Commit() (string, error) {
	return "", c.commit()
}

func (c *sqlDot) commit() error {
	if c.tx != nil {
		err := c.tx.Commit()
		c.log.Debug("commit", slog.Any("error", err))
		c.tx = nil
		return err
	}
	return nil
}

func (c *sqlDot) Rollback() (string, error) {
	return "", c.rollback()
}

func (c *sqlDot) rollback() error {
	if c.tx != nil {
		err := c.tx.Rollback()
		c.log.Debug("rollback", slog.Any("error", err))
		c.tx = nil
		return err
	}
	return nil
}
