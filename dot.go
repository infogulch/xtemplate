package xtemplate

import (
	"bytes"
	"context"
	"encoding"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sync"
)

var registrations map[string]RegisteredDotProvider = make(map[string]RegisteredDotProvider)

func RegisterDot(r RegisteredDotProvider) {
	name := r.Name()
	if old, ok := registrations[name]; ok {
		panic(fmt.Sprintf("DotProvider name already registered: %s (%v)", name, old))
	}
	registrations[name] = r
}

type DotConfig struct {
	Name string
	Type string
	DotProvider
}

type Request struct {
	DotConfig
	ServerCtx context.Context
	W         http.ResponseWriter
	R         *http.Request
}

type DotProvider interface {
	// Value must always return a valid instance of the same type, even if it
	// also returns an error. Value will be called with mock values at least
	// once and still must not panic.
	Value(Request) (any, error)
}

type RegisteredDotProvider interface {
	DotProvider
	Name() string
	New() DotProvider
}

type CleanupDotProvider interface {
	DotProvider
	Cleanup(any, error) error
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

func makeDot(dcs []DotConfig) dot {
	fields := make([]reflect.StructField, 0, len(dcs))
	cleanups := []cleanup{}
	mockHttpRequest := httptest.NewRequest("GET", "/", nil)
	for i, dc := range dcs {
		mockRequest := Request{dc, context.Background(), mockResponseWriter{}, mockHttpRequest}
		a, _ := dc.DotProvider.Value(mockRequest)
		t := reflect.TypeOf(a)
		if t.Kind() == reflect.Interface && t.NumMethod() == 0 {
			t = t.Elem()
		}
		f := reflect.StructField{
			Name:      dc.Name,
			Type:      t,
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
	return dot{dcs, cleanups, &sync.Pool{New: func() any { v := reflect.New(typ).Elem(); return &v }}}
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

func (d *dot) value(sctx context.Context, w http.ResponseWriter, r *http.Request) (val *reflect.Value, err error) {
	val = d.pool.Get().(*reflect.Value)
	val.SetZero()
	for i, dc := range d.dcs {
		var a any
		a, err = dc.Value(Request{dc, sctx, w, r})
		if err != nil {
			err = fmt.Errorf("failed to construct dot value for %s (%v): %w", dc.Name, dc.DotProvider, err)
			val.SetZero()
			d.pool.Put(val)
			val = nil
			return
		}
		val.Field(i).Set(reflect.ValueOf(a))
	}
	return
}

func (d *dot) cleanup(v *reflect.Value, err error) error {
	for _, cleanup := range d.cleanups {
		err = cleanup.Cleanup(v.Field(cleanup.idx).Interface(), err)
	}
	v.SetZero()
	d.pool.Put(v)
	return err
}

type mockResponseWriter struct{}

var _ http.ResponseWriter = mockResponseWriter{}

func (mockResponseWriter) Header() http.Header { return http.Header{} }

func (m mockResponseWriter) Write(b []byte) (int, error) {
	return 0, fmt.Errorf("this is a mock http.ResponseWriter")
}

func (m mockResponseWriter) WriteHeader(statusCode int) {}
