package xtemplate

import (
	"encoding/json"
	"fmt"
)

// registry maps type-string → constructor.
// Written only in init(), read-only afterward.
// ponytail: init()-only writes; race-free by Go's init happens-before guarantee.
// Add a sync.RWMutex if runtime registration becomes supported.
var registry = map[string]func() DotConfig{}

// Register makes a provider type available to resolveProviders. Call from init().
// Panics on duplicate registration (names the type in the message; the registering
// package is identified by the runtime's stack trace).
func Register(name string, ctor func() DotConfig) {
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("xtemplate: provider type %q already registered", name))
	}
	registry[name] = ctor
}

// resolveProviders decodes each raw JSON entry by peeking its "type" field,
// looking up the constructor, and re-decoding into the concrete type.
func resolveProviders(raw []json.RawMessage) ([]DotConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]DotConfig, 0, len(raw))
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
