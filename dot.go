package xtemplate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
)

type Request struct {
	DotConfig
	ServerCtx context.Context
	W         http.ResponseWriter
	R         *http.Request
}

type DotConfig interface {
	FieldName() string
	Init(context.Context) error
	Value(Request) (any, error)
}

type CleanupDotProvider interface {
	DotConfig
	Cleanup(any, error) error
}

func makeDot(dps []DotConfig) dot {
	fields := make([]reflect.StructField, 0, len(dps))
	cleanups := []cleanup{}
	mockHttpRequest := httptest.NewRequest("GET", "/", nil)
	for i, dp := range dps {
		mockRequest := Request{dp, context.Background(), mockResponseWriter{}, mockHttpRequest}
		a, _ := dp.Value(mockRequest)
		t := reflect.TypeOf(a)
		if t.Kind() == reflect.Interface && t.NumMethod() == 0 {
			t = t.Elem()
		}
		f := reflect.StructField{
			Name:      dp.FieldName(),
			Type:      t,
			Anonymous: false, // alas
		}
		fields = append(fields, f)
		if cdp, ok := dp.(CleanupDotProvider); ok {
			cleanups = append(cleanups, cleanup{i, cdp})
		}
	}
	typ := reflect.StructOf(fields)
	return dot{dps, cleanups, &sync.Pool{New: func() any { v := reflect.New(typ).Elem(); return &v }}}
}

type dot struct {
	dps      []DotConfig
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
	for i, dp := range d.dps {
		var a any
		a, err = dp.Value(Request{dp, sctx, w, r})
		if err != nil {
			err = fmt.Errorf("failed to construct dot value for %s (%v): %w", dp.FieldName(), dp, err)
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
