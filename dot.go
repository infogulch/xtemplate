package xtemplate

import (
	"bytes"
	"context"
	"encoding"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"slices"
	"sync"
)

/*

Name DotProvider

- Context providers should be able to add methods and fields directly to the
  root context {{.}}, by having an anonymous field
- Context providers should have customizable name

- Should have a default
- Users should be able to override name


Needs to get configuration from:

- Args
	- Parse args with https://pkg.go.dev/github.com/alexflint/go-arg#Parse
	- https://github.com/alexflint/go-arg/issues/220
	- Format:   -c "Tx:sql:sqlite3:file"
- Json
- Caddyfile
- Manual go configuration
- env? defaults?

*/

var registrations map[string]RegisteredDotProvider = make(map[string]RegisteredDotProvider)

func RegisterDot(r RegisteredDotProvider) {
	name := r.Name()
	if old, ok := registrations[name]; ok {
		panic(fmt.Sprintf("DotProvider name already registered: %s (%v)", name, old))
	}
	registrations[name] = r
}

type DotProvider interface {
	Type() reflect.Type
	Value(request_scoped_logger *slog.Logger, server_ctx context.Context, w http.ResponseWriter, r *http.Request) (reflect.Value, error)
}

type RegisteredDotProvider interface {
	DotProvider
	Name() string
	New() DotProvider
}

type CleanupDotProvider interface {
	DotProvider
	Cleanup(reflect.Value, error) error
}

type DotConfig struct {
	Name string
	DotProvider
}

func (d *DotConfig) UnmarshalText(b []byte) error {
	parts := bytes.SplitN(b, []byte{':'}, 3)
	if len(parts) < 2 {
		return fmt.Errorf("failed to parse DotConfig not enough sections. required format: NAME:PROVIDER_NAME[:PROVIDER_CONFIG]")
	}
	name, providerName := string(parts[0]), string(parts[1])
	reg, ok := registrations[providerName]
	if !ok {
		return fmt.Errorf("dot provider with name '%s' is not registered", providerName)
	}
	d.Name = name
	d.DotProvider = reg.New()
	if unm, ok := d.DotProvider.(encoding.TextUnmarshaler); ok {
		var rest []byte
		if len(parts) == 3 {
			rest = parts[2]
		}
		err := unm.UnmarshalText(rest)
		if err != nil {
			return fmt.Errorf("failed to configure provider %s: %w", providerName, err)
		}
	}
	return nil
}

func (d *DotConfig) MarshalText() ([]byte, error) {
	var parts [][]byte
	if r, ok := d.DotProvider.(RegisteredDotProvider); ok {
		parts = [][]byte{[]byte(d.Name), {':'}, []byte(r.Name())}
	} else {
		return nil, fmt.Errorf("dot provider cannot be marshalled: %v (%T)", d.DotProvider, d.DotProvider)
	}
	if m, ok := d.DotProvider.(encoding.TextMarshaler); ok {
		b, err := m.MarshalText()
		if err != nil {
			return nil, err
		}
		parts = append(parts, []byte{':'}, b)
	}
	return slices.Concat(parts...), nil
}

var _ encoding.TextUnmarshaler = &DotConfig{}
var _ encoding.TextMarshaler = &DotConfig{}

func makeDot(dcs []DotConfig) *dot {
	fields := make([]reflect.StructField, 0, len(dcs))
	cleanups := []cleanup{}
	for i, dc := range dcs {
		f := reflect.StructField{
			Name:      dc.Name,
			Type:      dc.DotProvider.Type(),
			Anonymous: false, // alas
		}
		if f.Name == "" {
			f.Name = f.Type.Name()
		}
		fields = append(fields, f)
		if cdp, ok := dc.DotProvider.(CleanupDotProvider); ok {
			cleanups = append(cleanups, cleanup{i, cdp})
		}
	}
	typ := reflect.StructOf(fields)
	return &dot{dcs, cleanups, &sync.Pool{New: func() any { v := reflect.New(typ).Elem(); return &v }}}
}

type dot struct {
	dcs      []DotConfig
	cleanups []cleanup
	pool     *sync.Pool
}

type cleanup struct {
	idx int
	CleanupDotProvider
}

func (d *dot) value(log *slog.Logger, sctx context.Context, w http.ResponseWriter, r *http.Request) (val *reflect.Value, err error) {
	val = d.pool.Get().(*reflect.Value)
	val.SetZero()
	for i, dc := range d.dcs {
		var v reflect.Value
		v, err = dc.Value(log, sctx, w, r)
		if err != nil {
			err = fmt.Errorf("failed to construct dot value for %s (%v): %w", dc.Name, dc.DotProvider, err)
			v.SetZero()
			d.pool.Put(val)
			val = nil
			return
		}
		val.Field(i).Set(v)
	}
	return
}

func (d *dot) cleanup(v *reflect.Value, err error) error {
	for _, cleanup := range d.cleanups {
		err = cleanup.Cleanup(v.Field(cleanup.idx), err)
	}
	v.SetZero()
	d.pool.Put(v)
	return err
}
