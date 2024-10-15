package xtemplate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func WithDB(name string, db *sql.DB, opt *sql.TxOptions) Option {
	return func(c *Config) error {
		if db == nil {
			return fmt.Errorf("cannot create database provider with nil sql.DB. name: %s", name)
		}
		c.Databases = append(c.Databases, DotDBConfig{Name: name, DB: db, TxOptions: opt})
		return nil
	}
}

type DotDBConfig struct {
	*sql.DB        `json:"-"`
	*sql.TxOptions `json:"-"`
	Name           string `json:"name"`
	Driver         string `json:"driver"`
	Connstr        string `json:"connstr"`
	MaxOpenConns   int    `json:"max_open_conns"`
}

var _ CleanupDotProvider = &DotDBConfig{}

func (d *DotDBConfig) FieldName() string { return d.Name }
func (d *DotDBConfig) Init(ctx context.Context) error {
	if d.DB != nil {
		return nil
	}
	db, err := sql.Open(d.Driver, d.Connstr)
	if err != nil {
		return fmt.Errorf("failed to open database with driver name '%s': %w", d.Driver, err)
	}
	db.SetMaxOpenConns(d.MaxOpenConns)
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database on open: %w", err)
	}
	d.DB = db
	return nil
}
func (d *DotDBConfig) Value(r Request) (any, error) {
	return &DotDB{d.DB, GetLogger(r.R.Context()), r.R.Context(), d.TxOptions, nil}, nil
}
func (dp *DotDBConfig) Cleanup(v any, err error) error {
	d := v.(*DotDB)
	if err != nil {
		return errors.Join(err, d.rollback())
	} else {
		return errors.Join(err, d.commit())
	}
}
