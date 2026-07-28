package xtemplate

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
)

// ServerController is attached once per Server by [Config.Server]:
//
//   - Init returns sticky [Option]s for the base config (no Server methods).
//   - Start may drive reloads via server.Reload (safe to call synchronously).
//
// Built-in: os. Optional: controllers/watchfs, controllers/git.
type ServerController interface {
	Init(ctx context.Context, log *slog.Logger) (sticky []Option, err error)
	Start(server *Server) error
}

// DefaultControllerType is used when no controller or TemplateFS is configured.
// Core default is "os". Binaries may reassign (e.g. cmd/xtemplate sets "watchfs").
var DefaultControllerType = "os"

// controllerRegistry maps type-string → constructor.
// Written only in init(), read-only afterward.
var controllerRegistry = map[string]func() ServerController{}

// RegisterController makes a controller type available to ResolveController.
// Call from init(). Panics on duplicate registration.
func RegisterController(name string, ctor func() ServerController) {
	if _, exists := controllerRegistry[name]; exists {
		panic(fmt.Sprintf("xtemplate: controller type %q already registered", name))
	}
	controllerRegistry[name] = ctor
}

// RegisteredControllerTypes returns sorted registered controller type names.
func RegisteredControllerTypes() []string {
	return slices.Sorted(maps.Keys(controllerRegistry))
}

// unknownControllerType returns a registry-miss error with import hints for known names.
func unknownControllerType(name string) error {
	switch name {
	case "os":
		return fmt.Errorf("xtemplate: unknown controller type %q; built-in controllers should be registered by the core package", name)
	case "watchfs", "git":
		return fmt.Errorf("xtemplate: unknown controller type %q; add it by importing github.com/infogulch/xtemplate/controllers/%s", name, name)
	default:
		return fmt.Errorf("xtemplate: unknown controller type %q; ensure the controller package that registers type %q is imported", name, name)
	}
}

// NewController returns a new zero-value ServerController for a registered type name.
func NewController(name string) (ServerController, error) {
	ctor, ok := controllerRegistry[name]
	if !ok {
		return nil, unknownControllerType(name)
	}
	return ctor(), nil
}

// ControllerTypeFromRaw peeks the "type" field of a controller JSON object.
// Empty raw returns ("", nil).
func ControllerTypeFromRaw(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return "", fmt.Errorf("xtemplate: failed to read controller type: %w", err)
	}
	return probe.Type, nil
}

// ResolveController decodes raw JSON by peeking its "type" field, looking up the
// constructor, and re-decoding into the concrete type.
func ResolveController(raw json.RawMessage) (ServerController, error) {
	typ, err := ControllerTypeFromRaw(raw)
	if err != nil {
		return nil, err
	}
	if typ == "" {
		return nil, fmt.Errorf("xtemplate: controller JSON missing \"type\" field")
	}
	s, err := NewController(typ)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, s); err != nil {
		return nil, fmt.Errorf("xtemplate: failed to decode controller %q config: %w", typ, err)
	}
	return s, nil
}

// MaterializeController ensures c.Controller is set and c.ControllerRaw is nil.
// The returned string is the selected type name when known (from Raw, preferType,
// or DefaultControllerType). Empty when Controller was already set (name unknown).
//
// If Controller is already non-nil, only clears ControllerRaw.
// If ControllerRaw is set:
//   - When preferType is empty (library/Server), always decode Raw.
//   - When preferType is non-empty (CLI effective type), decode Raw only if its
//     type is empty or equals preferType; otherwise discard Raw (CLI type wins).
//     An empty raw type still attempts decode so ResolveController reports
//     "missing type".
// If still no Controller, creates NewController(preferType), or
// DefaultControllerType when preferType is empty.
func (c *Config) MaterializeController(preferType string) (string, error) {
	if c.Controller != nil {
		c.ControllerRaw = nil
		return "", nil
	}

	if len(c.ControllerRaw) > 0 {
		rawType, err := ControllerTypeFromRaw(c.ControllerRaw)
		if err != nil {
			return "", err
		}
		useRaw := preferType == "" || rawType == "" || rawType == preferType
		if useRaw {
			ctrl, err := ResolveController(c.ControllerRaw)
			if err != nil {
				return "", err
			}
			c.Controller = ctrl
			c.ControllerRaw = nil
			return rawType, nil
		}
		c.ControllerRaw = nil
	}

	name := preferType
	if name == "" {
		name = DefaultControllerType
	}
	ctrl, err := NewController(name)
	if err != nil {
		return "", err
	}
	c.Controller = ctrl
	return name, nil
}
