package xtemplate

import (
	"fmt"
	"net/http"
)

type dotVarsProvider struct{}

func (dotVarsProvider) FieldName() string { return "Vars" }
func (dotVarsProvider) Prototype() any    { return DotVars{} }
func (dotVarsProvider) Value(http.ResponseWriter, *http.Request) (any, error) {
	// Non-nil empty map every request. A nil map panics under Sprig set and
	// assignment; concurrent requests must not share a map.
	return make(DotVars), nil
}

var _ Provider = dotVarsProvider{}

// DotVars is the per-request scratch map at .Vars. It is map-shaped so FuncMaps
// that work on `map[string]any` work with it.
type DotVars map[string]any

// Set stores value under key. key must be non-empty. Returns an empty string
// so it can be used as {{.Vars.Set "list" $row}} or {{$_ := .Vars.Set …}}.
func (v DotVars) Set(key string, value any) (string, error) {
	if key == "" {
		return "", fmt.Errorf("Vars.Set: key must be non-empty")
	}
	v[key] = value
	return "", nil
}

// Get returns the value for key, or nil if the key is missing.
// key must be non-empty.
func (v DotVars) Get(key string) (any, error) {
	if key == "" {
		return nil, fmt.Errorf("Vars.Get: key must be non-empty")
	}
	return v[key], nil
}

// Has reports whether key is present. key must be non-empty.
func (v DotVars) Has(key string) (bool, error) {
	if key == "" {
		return false, fmt.Errorf("Vars.Has: key must be non-empty")
	}
	_, ok := v[key]
	return ok, nil
}

// Delete removes key if present. key must be non-empty. Returns an empty string.
func (v DotVars) Delete(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("Vars.Delete: key must be non-empty")
	}
	delete(v, key)
	return "", nil
}
