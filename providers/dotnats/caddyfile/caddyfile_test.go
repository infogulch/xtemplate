package caddyfile_test

import (
	"encoding/json"
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"

	xtc "github.com/infogulch/xtemplate/caddy"
	_ "github.com/infogulch/xtemplate/providers/dotnats/caddyfile"
)

func parse(t *testing.T, body string) (json.RawMessage, error) {
	t.Helper()
	input := "outer {\n\tprovider nats Nats {\n" + body + "\n\t}\n}"
	h := httpcaddyfile.Helper{}.WithDispenser(caddyfile.NewTestDispenser(input))
	h.Next()       // "outer"
	h.NextBlock(0) // → "provider"
	h.NextArg()    // "nats"
	h.NextArg()    // "Nats"
	mi, err := caddy.GetModule("xtemplate.providers.nats")
	if err != nil {
		t.Fatalf("GetModule: %v", err)
	}
	return mi.New().(xtc.CaddyfileBlockParser).ParseCaddyfile(h)
}

func TestNatsCaddyfile_InProcessServer(t *testing.T) {
	raw, err := parse(t, "\t\tin_process_server {\n\t\t\tdont_listen true\n\t\t}")
	if err != nil {
		t.Fatalf("ParseCaddyfile: %v", err)
	}
	var got struct {
		NatsConfig struct {
			InProcessServer *struct {
				DontListen bool `json:"dont_listen"`
			} `json:"in_process_server_options"`
		} `json:"nats_config"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.NatsConfig.InProcessServer == nil {
		t.Fatal("in_process_server_options = nil, want non-nil")
	}
	if !got.NatsConfig.InProcessServer.DontListen {
		t.Error("dont_listen = false, want true")
	}
}

func TestNatsCaddyfile_ConnOptions(t *testing.T) {
	raw, err := parse(t, "\t\tconn_options {\n\t\t\turl nats://localhost:4222\n\t\t}")
	if err != nil {
		t.Fatalf("ParseCaddyfile: %v", err)
	}
	var got struct {
		NatsConfig struct {
			ConnOptions *struct {
				Url string `json:"Url"`
			} `json:"conn_options"`
		} `json:"nats_config"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.NatsConfig.ConnOptions == nil {
		t.Fatal("conn_options = nil, want non-nil")
	}
	if got.NatsConfig.ConnOptions.Url != "nats://localhost:4222" {
		t.Errorf("url = %q, want nats://localhost:4222", got.NatsConfig.ConnOptions.Url)
	}
}

func TestNatsCaddyfile_EmptyBlock(t *testing.T) {
	raw, err := parse(t, "")
	if err != nil {
		t.Fatalf("ParseCaddyfile: %v", err)
	}
	var got struct {
		NatsConfig struct {
			InProcessServer interface{} `json:"in_process_server_options"`
			ConnOptions     interface{} `json:"conn_options"`
		} `json:"nats_config"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.NatsConfig.InProcessServer != nil {
		t.Errorf("in_process_server_options = %v, want nil", got.NatsConfig.InProcessServer)
	}
	if got.NatsConfig.ConnOptions != nil {
		t.Errorf("conn_options = %v, want nil", got.NatsConfig.ConnOptions)
	}
}

func TestNatsCaddyfile_Errors(t *testing.T) {
	cases := map[string]string{
		"unknown top-level key":     "\t\tbogus val",
		"unknown in_process_server": "\t\tin_process_server {\n\t\t\tbogus val\n\t\t}",
		"non-bool dont_listen":      "\t\tin_process_server {\n\t\t\tdont_listen notabool\n\t\t}",
		"unknown conn_options key":  "\t\tconn_options {\n\t\t\tbogus val\n\t\t}",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parse(t, body); err == nil {
				t.Errorf("expected error for %q", name)
			}
		})
	}
}
