package xtemplate

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// DotDB is used to create a dot field value that can query a SQL database. When
// any of its statement executing methods are called, it creates a new
// transaction. When template execution finishes, if there were no errors it
// automatically commits any uncommitted transactions remaining after template
// execution completes, but if there were errors then it calls rollback on the
// transaction.
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

// Exec executes a statement with parameters and returns the raw [sql.Result].
// Note: this can be a bit difficult to use inside a template, consider using
// other methods that provide easier to use return values.
func (c *DotDB) Exec(query string, params ...any) (result sql.Result, err error) {
	if err = c.makeTx(); err != nil {
		return
	}

	defer func(start time.Time) {
		c.log.Debug("Exec", slog.String("query", query), slog.Any("params", params), slog.Any("error", err), slog.Duration("queryduration", time.Since(start)))
	}(time.Now())

	return c.tx.Exec(query, params...)
}

// QueryRows executes a query and buffers all rows into a []map[string]any object.
func (c *DotDB) QueryRows(query string, params ...any) (rows []map[string]any, err error) {
	if err = c.makeTx(); err != nil {
		return
	}

	defer func(start time.Time) {
		c.log.Debug("QueryRows", slog.String("query", query), slog.Any("params", params), slog.Any("error", err), slog.Duration("queryduration", time.Since(start)))
	}(time.Now())

	result, err := c.tx.Query(query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
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

// QueryRow executes a query, which must return one row, and returns it as a
// map[string]any.
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

// QueryVal executes a query, which must return one row with one column, and
// returns the value of the column.
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

// Commit manually commits any implicit transactions opened by this DotDB. This
// is called automatically if there were no errors at the end of template
// execution.
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

// Rollback manually rolls back any implicit tranactions opened by this DotDB.
// This is called automatically if there were any errors that occurred during
// template exeuction.
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
