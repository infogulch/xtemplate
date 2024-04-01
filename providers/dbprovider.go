package providers

import (
	"database/sql"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/infogulch/xtemplate"
)

func init() {
	xtemplate.RegisterDot(&DotDBProvider{})
}

func WithDB(name string, db *sql.DB, opt *sql.TxOptions) xtemplate.ConfigOverride {
	if db == nil {
		panic(fmt.Sprintf("cannot to create DotDBProvider with null DB with name %s", name))
	}
	return xtemplate.WithProvider(name, &DotDBProvider{DB: db, TxOptions: opt})
}

type DotDBProvider struct {
	*sql.DB        `json:"-"`
	*sql.TxOptions `json:"-"`
	Driver         string `json:"driver"`
	Connstr        string `json:"connstr"`
}

var _ encoding.TextMarshaler = &DotDBProvider{}

func (d *DotDBProvider) MarshalText() ([]byte, error) {
	if d.Driver == "" || d.Connstr == "" {
		return nil, fmt.Errorf("cannot unmarshal because SqlDot does not have the driver and connstr")
	}
	return []byte(d.Driver + ":" + d.Connstr), nil
}

var _ encoding.TextUnmarshaler = &DotDBProvider{}

func (d *DotDBProvider) UnmarshalText(b []byte) error {
	parts := strings.SplitN(string(b), ":", 2)
	if len(parts) < 2 {
		return fmt.Errorf("not enough parameters to configure sql dot. Requires DRIVER:CONNSTR, got: %s", string(b))
	}
	d.Driver = parts[0]
	d.Connstr = parts[1]
	return nil
}

var _ json.Marshaler = &DotDBProvider{}

func (d *DotDBProvider) MarshalJSON() ([]byte, error) {
	type T DotDBProvider
	return json.Marshal((*T)(d))
}

var _ json.Unmarshaler = &DotDBProvider{}

func (d *DotDBProvider) UnmarshalJSON(b []byte) error {
	type T DotDBProvider
	return json.Unmarshal(b, (*T)(d))
}

var _ xtemplate.CleanupDotProvider = &DotDBProvider{}

func (DotDBProvider) New() xtemplate.DotProvider { return &DotDBProvider{} }
func (DotDBProvider) Type() string               { return "sql" }
func (d *DotDBProvider) Value(r xtemplate.Request) (any, error) {
	if d.DB == nil {
		db, err := sql.Open(d.Driver, d.Connstr)
		if err != nil {
			return &DotDB{}, fmt.Errorf("failed to open database with driver name '%s': %w", d.Driver, err)
		}
		if err := db.Ping(); err != nil {
			return &DotDB{}, fmt.Errorf("failed to ping database on open: %w", err)
		}
		d.DB = db
	}
	return &DotDB{d.DB, xtemplate.GetCtxLogger(r.R), r.R.Context(), d.TxOptions, nil}, nil
}

func (dp *DotDBProvider) Cleanup(v any, err error) error {
	d := v.(*DotDB)
	if err != nil {
		return errors.Join(err, d.rollback())
	} else {
		return errors.Join(err, d.commit())
	}
}
