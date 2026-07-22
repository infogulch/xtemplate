package xtemplate

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/spf13/afero"
)

// TemplateSource supplies the template filesystem for a Server (or static Instance)
// and optionally a channel of reload options when content changes.
//
// Start is called once per Server (or once for a standalone Instance). ctx is the
// server context and must not leak work past cancel. initial may be nil to mean
// "not ready yet" — the Server installs a placeholder MemMapFs containing
// `.any.html` with `{{define "ANY /"}}…503…` until a reload brings real content.
// When Start returns nil initial, every [Server.Reload] must include
// [WithTemplateFS] or [WithTemplateDir] (reload options are not sticky; the
// base FS stays the placeholder).
//
// A non-nil reloads channel is Server-only; standalone Instance rejects it.
type TemplateSource interface {
	Start(ctx context.Context, log *slog.Logger) (initial afero.Fs, reloads <-chan []Option, err error)
}

// sourceRegistry maps type-string → constructor.
// Written only in init(), read-only afterward.
var sourceRegistry = map[string]func() TemplateSource{}

// RegisterSource makes a template source type available to ResolveSource. Call from init().
// Panics on duplicate registration.
func RegisterSource(name string, ctor func() TemplateSource) {
	if _, exists := sourceRegistry[name]; exists {
		panic(fmt.Sprintf("xtemplate: source type %q already registered", name))
	}
	sourceRegistry[name] = ctor
}

// RegisteredSourceTypes returns sorted names of registered source types (for CLI help).
func RegisteredSourceTypes() []string {
	return slices.Sorted(maps.Keys(sourceRegistry))
}

// NewSource returns a new zero-value TemplateSource for a registered type name.
func NewSource(name string) (TemplateSource, error) {
	ctor, ok := sourceRegistry[name]
	if !ok {
		return nil, fmt.Errorf("xtemplate: unknown source type %q", name)
	}
	return ctor(), nil
}

// ResolveSource decodes raw JSON by peeking its "type" field, looking up the
// constructor, and re-decoding into the concrete type.
func ResolveSource(raw json.RawMessage) (TemplateSource, error) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("xtemplate: failed to read source type: %w", err)
	}
	if probe.Type == "" {
		return nil, fmt.Errorf("xtemplate: source JSON missing \"type\" field")
	}
	ctor, ok := sourceRegistry[probe.Type]
	if !ok {
		switch probe.Type {
		case "os":
			return nil, fmt.Errorf("xtemplate: unknown source type %q; built-in sources should be registered by the core package", probe.Type)
		case "watchfs", "git":
			return nil, fmt.Errorf("xtemplate: unknown source type %q; add it by importing github.com/infogulch/xtemplate/sources/%s", probe.Type, probe.Type)
		default:
			return nil, fmt.Errorf("xtemplate: unknown source type %q; ensure the source package that registers type %q is imported", probe.Type, probe.Type)
		}
	}
	s := ctor()
	if err := json.Unmarshal(raw, s); err != nil {
		return nil, fmt.Errorf("xtemplate: failed to decode source %q config: %w", probe.Type, err)
	}
	return s, nil
}

// notReadyTemplatesFS is the placeholder FS used when TemplateSource.Start returns nil.
var notReadyTemplatesFS afero.Fs = (func() afero.Fs {
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, ".any.html", []byte(`{{define "ANY /"}}{{.Resp.ReturnStatus 503}}{{end}}`), 0o644); err != nil {
		// MemMapFs writes do not fail under normal conditions; keep a hard fallback.
		panic("xtemplate: notReadyTemplatesFS: " + err.Error())
	}
	return fs
})()

// bannedTemplateKeys are legacy top-level Config JSON keys that hard-reject.
// REMOVE BEFORE 1.0: temporary migration hard-rejects for renamed top-level keys.
// After 1.0 these keys are simply unknown and ignored like any other typo.
var bannedTemplateKeys = map[string]string{
	"templates_dir":       `templates_dir is no longer supported; use "source": {"type":"os","path":"…"} (or watchfs/git as appropriate)`,
	"templates_path":      `templates_path is no longer supported; use "source": {"type":"os","path":"…"} (or watchfs/git as appropriate)`,
	"watch_dirs":          `watch_dirs is no longer supported; use "source": {"type":"watchfs","path":"…","watch_dirs":[…]}`,
	"watch_template_path": `watch_template_path is no longer supported; use "source": {"type":"watchfs",…} or omit for default os`,
	"git_repo":            `git_repo is no longer supported; use "source": {"type":"git","repo":"…"}`,
	"git_ref":             `git_ref is no longer supported; use "source": {"type":"git","ref":"…"}`,
	"git_interval":        `git_interval is no longer supported; use "source": {"type":"git","interval":"…"}`,
}

// CheckLegacyTemplateKeys probes raw JSON for banned top-level keys and returns
// a migrate error if any are present. Other unknown keys are still ignored by
// normal Unmarshal. Call before json.Unmarshal into Config.
//
// REMOVE BEFORE 1.0
func CheckLegacyTemplateKeys(data []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		// Let the real Unmarshal report structural errors.
		return nil
	}
	for key, msg := range bannedTemplateKeys {
		if _, ok := m[key]; ok {
			return fmt.Errorf("xtemplate: %s", msg)
		}
	}
	return nil
}
