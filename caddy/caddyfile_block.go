package xtemplate_caddy

import (
	"encoding/json"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

// CaddyfileBlockParser is implemented by Caddy modules in the
// "xtemplate.providers.*" namespace that want to expose Caddyfile block syntax.
// ParseCaddyfile must return a JSON object containing only the provider's
// type-specific fields. The "type" and "name" keys are reserved - the dispatch
// injects them; returning either is a contract violation surfaced at parse time.
type CaddyfileBlockParser interface {
	ParseCaddyfile(h httpcaddyfile.Helper) (json.RawMessage, error)
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

	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return h.Errf("provider %s: parser returned invalid JSON: %v", typeName, err)
	}
	if _, ok := m["type"]; ok {
		return h.Errf("provider %s parser must not emit a 'type' key; it is injected by the dispatch", typeName)
	}
	if _, ok := m["name"]; ok {
		return h.Errf("provider %s parser must not emit a 'name' key; it is injected by the dispatch", typeName)
	}

	m["type"], _ = json.Marshal(typeName)
	m["name"], _ = json.Marshal(fieldName)
	final, err := json.Marshal(m)
	if err != nil {
		return err
	}
	t.ProvidersRaw = append(t.ProvidersRaw, final)
	return nil
}
