package xtemplate

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"sync"
)

// Provider contributes one named field on the per-request dot context.
//
// Lifecycle:
//   - [FieldName] and [Prototype] are used when an instance is built.
//   - Optional [Initializer.Init] runs once per instance load (after config
//     decode, before Prototype is used for type assembly). Save the instance
//     context there if request-time code must observe reload/stop.
//   - [Value] runs once per request (and for INIT templates).
//   - Optional [Finalizer.Finalize] runs after template execution.
//   - Optional [Closer.Close] runs when the instance is retired (reload/stop).
//
// Optional hooks are separate capability interfaces (like [http.Flusher]); they
// do not embed [Provider]. Assert them when needed.
type Provider interface {
	// FieldName is the exported struct field on the dot (e.g. "Shop" → {{.Shop}}).
	FieldName() string
	// Prototype returns a non-nil value of the field type. It is called once
	// when building the instance solely to infer the type via reflection; the
	// returned value is discarded. It must not depend on request data.
	Prototype() any
	// Value returns the value to assign to this provider's field for a request.
	// w and r are the HTTP response writer and request (same as http.Handler).
	// Request-scoped context is r.Context(); instance lifetime context should
	// have been saved during [Initializer.Init] if needed.
	Value(w http.ResponseWriter, r *http.Request) (any, error)
}

// Initializer is an optional capability for instance-scoped setup (open DB,
// connect, validate config, etc.). Init runs once per instance load with the
// instance context (cancelled on reload/stop). Providers that need that context
// at request time should retain it on the provider value.
type Initializer interface {
	Init(context.Context) error
}

// Finalizer is an optional capability for work after template execution
// (commit/rollback, close request-scoped handles, write buffered response
// headers/status). value is what Value returned for the request; err is the
// template/construction error so far. The returned error replaces err for
// subsequent finalizers and the handler.
type Finalizer interface {
	Finalize(value any, err error) error
}

// Closer is an optional capability for releasing instance-scoped resources when
// the instance is retired (reload or stop). Prefer Close over relying solely on
// context cancellation when the provider owns connections or similar handles.
type Closer interface {
	Close() error
}

func makeDot(dps []Provider) (dot, error) {
	fields := make([]reflect.StructField, 0, len(dps))
	finalizers := []finalizer{}
	for i, dp := range dps {
		a := dp.Prototype()
		t := reflect.TypeOf(a)
		if t == nil {
			return dot{}, fmt.Errorf("dot provider %q (%T) Prototype returned nil; Prototype must return a non-nil typed value", dp.FieldName(), dp)
		}
		f := reflect.StructField{
			Name:      dp.FieldName(),
			Type:      t,
			Anonymous: false, // alas
		}
		fields = append(fields, f)
		if fdp, ok := dp.(Finalizer); ok {
			finalizers = append(finalizers, finalizer{i, fdp})
		}
	}
	typ := reflect.StructOf(fields)
	return dot{dps, finalizers, &sync.Pool{New: func() any { v := reflect.New(typ).Elem(); return &v }}}, nil
}

type dot struct {
	dps        []Provider
	finalizers []finalizer
	pool       *sync.Pool
}

type finalizer struct {
	idx int
	Finalizer
}

func (d *dot) value(w http.ResponseWriter, r *http.Request) (val *reflect.Value, err error) {
	val = d.pool.Get().(*reflect.Value)
	val.SetZero()
	for i, dp := range d.dps {
		var a any
		a, err = dp.Value(w, r)
		if err != nil {
			err = fmt.Errorf("failed to construct dot value for %s (%v): %w", dp.FieldName(), dp, err)
			// Unwind the providers that were already constructed for this request
			// (only fields before index i have been set) so they don't leak
			// resources, e.g. an open DB transaction whose Finalize rolls back.
			err = finalizeSlice(d.finalizers[:i], val, err)
			val.SetZero()
			d.pool.Put(val)
			val = nil
			return
		}
		val.Field(i).Set(reflect.ValueOf(a))
	}
	return
}

// finalizeSlice runs Finalize for every provided Finalizer, folding any
// finalize errors into err. d.finalizers is ordered by ascending field index,
// so it can stop early on partial unwind.
func finalizeSlice(finalizers []finalizer, v *reflect.Value, err error) error {
	for _, f := range finalizers {
		err = f.Finalize(v.Field(f.idx).Interface(), err)
	}
	return err
}

func (d *dot) cleanup(v *reflect.Value, err error) error {
	err = finalizeSlice(d.finalizers, v, err)
	v.SetZero()
	d.pool.Put(v)
	return err
}
