package xtemplate_caddy

import (
	"encoding/json"
	"fmt"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

// CaddyfileBlockParser is implemented by Caddy modules in the
// "xtemplate.providers.*" and "xtemplate.source.*" namespaces that want to
// expose Caddyfile block syntax. ParseCaddyfile must return a JSON object
// containing only the type-specific fields. Reserved keys are injected by the
// dispatch (providers: "type" and "name"; sources: "type").
type CaddyfileBlockParser interface {
	ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error)
}

// injectReserved unmarshals parser JSON, rejects any key present in inject,
// then injects those keys and re-marshals.
func injectReserved(raw json.RawMessage, inject map[string]any) (json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parser returned invalid JSON: %w", err)
	}
	for k := range inject {
		if _, ok := m[k]; ok {
			return nil, fmt.Errorf("parser must not emit a %q key; it is injected by the dispatch", k)
		}
	}
	for k, v := range inject {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		m[k] = b
	}
	return json.Marshal(m)
}

// parseProviderBlock handles a `provider <type> <field> { }` block inside
// parseCaddyfile. It looks up the named Caddy module, asserts CaddyfileBlockParser,
// calls ParseCaddyfile, injects "type" and "name", and appends to ProvidersRaw.
func parseProviderBlock(h httpcaddyfile.Helper, t *XTemplateModule) error {
	if !h.NextArg() {
		return h.Errf("provider requires a type and field name")
	}
	typeName := h.Val()

	if !h.NextArg() {
		return h.Errf("provider %s requires a field name", typeName)
	}
	fieldName := h.Val()

	mi, err := caddy.GetModule("xtemplate.providers." + typeName)
	if err != nil {
		return h.Errf("provider type %q is not available in this build; "+
			"add it with --with github.com/infogulch/xtemplate/providers/%s/caddyfile",
			typeName, typeName)
	}
	parser, ok := mi.New().(CaddyfileBlockParser)
	if !ok {
		return h.Errf("module xtemplate.providers.%s does not implement CaddyfileBlockParser", typeName)
	}

	raw, err := parser.ParseCaddyfile(h)
	if err != nil {
		return err
	}

	final, err := injectReserved(raw, map[string]any{"type": typeName, "name": fieldName})
	if err != nil {
		return h.Errf("provider %s: %v", typeName, err)
	}
	t.ProvidersRaw = append(t.ProvidersRaw, final)
	return nil
}

// parseSourceBlock handles a `source <type> { }` block inside parseCaddyfile.
// It looks up xtemplate.source.<type>, asserts CaddyfileBlockParser, injects
// "type", and sets SourceRaw (one source per module).
func parseSourceBlock(h httpcaddyfile.Helper, t *XTemplateModule) error {
	if !h.NextArg() {
		return h.Errf("source requires a type")
	}
	typeName := h.Val()

	if len(t.SourceRaw) > 0 {
		return h.Errf("only one source block is allowed")
	}

	mi, err := caddy.GetModule("xtemplate.source." + typeName)
	if err != nil {
		return h.Errf("source type %q is not available in this build; "+
			"add it with --with github.com/infogulch/xtemplate/sources/%s/caddyfile "+
			"(built-in os is in xtemplate/caddy)",
			typeName, typeName)
	}
	parser, ok := mi.New().(CaddyfileBlockParser)
	if !ok {
		return h.Errf("module xtemplate.source.%s does not implement CaddyfileBlockParser", typeName)
	}

	raw, err := parser.ParseCaddyfile(h)
	if err != nil {
		return err
	}

	final, err := injectReserved(raw, map[string]any{"type": typeName})
	if err != nil {
		return h.Errf("source %s: %v", typeName, err)
	}
	t.SourceRaw = final
	return nil
}
