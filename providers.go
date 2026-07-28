package xtemplate

import (
	"encoding/json"
	"fmt"
)

// ProviderFactory constructs a fresh [Provider] value. Sticky [Config.Providers]
// hold factories so each Instance / Reload build materializes new providers
// (matching JSON [resolveProviders] + registry constructors).
type ProviderFactory func() Provider

// registry maps type-string → constructor.
// Written only in init(), read-only afterward.
// ponytail: init()-only writes; race-free by Go's init happens-before guarantee.
// Add a sync.RWMutex if runtime registration becomes supported.
var registry = map[string]ProviderFactory{}

// RegisterProvider makes a provider type available to resolveProviders. Call from init().
// Panics on duplicate registration (names the type in the message; the registering
// package is identified by the runtime's stack trace).
func RegisterProvider(name string, ctor ProviderFactory) {
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("xtemplate: provider type %q already registered", name))
	}
	registry[name] = ctor
}

// resolveProviders decodes each raw JSON entry by peeking its "type" field,
// looking up the constructor, and re-decoding into the concrete type.
func resolveProviders(raw []json.RawMessage) ([]Provider, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]Provider, 0, len(raw))
	for _, msg := range raw {
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(msg, &probe); err != nil {
			return nil, fmt.Errorf("xtemplate: failed to read provider type: %w", err)
		}
		ctor, ok := registry[probe.Type]
		if !ok {
			switch probe.Type {
			case "sql", "fs", "flags", "nats", "smtp", "bus":
				return nil, fmt.Errorf("xtemplate: unknown provider type %q; add it by importing github.com/infogulch/xtemplate/providers/dot%s", probe.Type, probe.Type)
			default:
				return nil, fmt.Errorf("xtemplate: unknown provider type %q; ensure the provider package that registers type %q is imported", probe.Type, probe.Type)
			}
		}
		p := ctor()
		if err := json.Unmarshal(msg, p); err != nil {
			return nil, fmt.Errorf("xtemplate: failed to decode provider %q config: %w", probe.Type, err)
		}
		out = append(out, p)
	}
	return out, nil
}

// materializeFactories invokes each sticky Go [ProviderFactory] for one Instance
// build. Rejects a nil factory or a factory that returns nil.
func materializeFactories(fs []ProviderFactory) ([]Provider, error) {
	if len(fs) == 0 {
		return nil, nil
	}
	out := make([]Provider, 0, len(fs))
	for i, f := range fs {
		if f == nil {
			return nil, fmt.Errorf("xtemplate: nil provider factory at index %d", i)
		}
		p := f()
		if p == nil {
			return nil, fmt.Errorf("xtemplate: provider factory at index %d returned nil", i)
		}
		out = append(out, p)
	}
	return out, nil
}
