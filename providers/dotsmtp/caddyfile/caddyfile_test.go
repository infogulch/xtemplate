package caddyfile_test

import (
	"encoding/json"
	"testing"

	caddy "github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"

	xtc "github.com/infogulch/xtemplate/caddy"
	_ "github.com/infogulch/xtemplate/providers/dotsmtp/caddyfile"
)

func parse(t *testing.T, body string) (json.RawMessage, error) {
	t.Helper()
	input := "outer {\n\tprovider smtp Email {\n" + body + "\n\t}\n}"
	h := httpcaddyfile.Helper{}.WithDispenser(caddyfile.NewTestDispenser(input))
	h.Next()       // "outer"
	h.NextBlock(0) // → "provider"
	h.NextArg()    // "smtp"
	h.NextArg()    // "Email"
	mi, err := caddy.GetModule("xtemplate.providers.smtp")
	if err != nil {
		t.Fatalf("GetModule: %v", err)
	}
	return mi.New().(xtc.CaddyfileBlockParser).ParseCaddyfile(h)
}

func TestSMTPCaddyfile_Full(t *testing.T) {
	raw, err := parse(t, "\t\thost smtp.example.com\n\t\tport 465\n\t\tusername me\n\t\tpassword secret\n\t\tauth plain\n\t\ttls tls\n\t\tfrom noreply@example.com\n\t\tmax_recipients 25\n\t\tmax_message_bytes 2097152\n\t\tsend_timeout 45s")
	if err != nil {
		t.Fatalf("ParseCaddyfile: %v", err)
	}
	var got struct {
		Host            string `json:"host"`
		Port            int    `json:"port"`
		Username        string `json:"username"`
		Password        string `json:"password"`
		Auth            string `json:"auth"`
		TLS             string `json:"tls"`
		From            string `json:"from"`
		MaxRecipients   int    `json:"max_recipients"`
		MaxMessageBytes int64  `json:"max_message_bytes"`
		SendTimeout     string `json:"send_timeout"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Host != "smtp.example.com" {
		t.Errorf("host = %q", got.Host)
	}
	if got.Port != 465 {
		t.Errorf("port = %d, want 465", got.Port)
	}
	if got.Username != "me" {
		t.Errorf("username = %q", got.Username)
	}
	if got.Password != "secret" {
		t.Errorf("password = %q", got.Password)
	}
	if got.Auth != "plain" {
		t.Errorf("auth = %q", got.Auth)
	}
	if got.TLS != "tls" {
		t.Errorf("tls = %q", got.TLS)
	}
	if got.From != "noreply@example.com" {
		t.Errorf("from = %q", got.From)
	}
	if got.MaxRecipients != 25 {
		t.Errorf("max_recipients = %d, want 25", got.MaxRecipients)
	}
	if got.MaxMessageBytes != 2097152 {
		t.Errorf("max_message_bytes = %d, want 2097152", got.MaxMessageBytes)
	}
	if got.SendTimeout != "45s" {
		t.Errorf("send_timeout = %q, want 45s", got.SendTimeout)
	}
}

func TestSMTPCaddyfile_SendTimeoutDuration(t *testing.T) {
	raw, err := parse(t, "\t\thost smtp.example.com\n\t\tfrom noreply@example.com\n\t\tsend_timeout 1m30s")
	if err != nil {
		t.Fatalf("ParseCaddyfile: %v", err)
	}
	var got struct {
		SendTimeout string `json:"send_timeout"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SendTimeout != "1m30s" {
		t.Errorf("send_timeout = %q, want 1m30s", got.SendTimeout)
	}
}

func TestSMTPCaddyfile_EmptyBlock(t *testing.T) {
	if _, err := parse(t, ""); err == nil {
		t.Fatal("expected error for empty block (host/from required), got nil")
	}
}

func TestSMTPCaddyfile_MissingFrom(t *testing.T) {
	if _, err := parse(t, "\t\thost smtp.example.com"); err == nil {
		t.Fatal("expected error for missing from, got nil")
	}
}

func TestSMTPCaddyfile_MissingHost(t *testing.T) {
	if _, err := parse(t, "\t\tfrom noreply@example.com"); err == nil {
		t.Fatal("expected error for missing host, got nil")
	}
}

func TestSMTPCaddyfile_NoTypeOrNameKeys(t *testing.T) {
	// The dispatch injects "type" and "name"; the parser must not emit them.
	raw, err := parse(t, "\t\thost smtp.example.com\n\t\tfrom noreply@example.com")
	if err != nil {
		t.Fatalf("ParseCaddyfile: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["type"]; ok {
		t.Error("parser emitted reserved 'type' key")
	}
	if _, ok := m["name"]; ok {
		t.Error("parser emitted reserved 'name' key")
	}
}

func TestSMTPCaddyfile_Errors(t *testing.T) {
	cases := map[string]string{
		"unknown key":          "\t\tbogus val",
		"non-int port":         "\t\thost h\n\t\tfrom f@x.com\n\t\tport notanint",
		"non-int max_rec":      "\t\thost h\n\t\tfrom f@x.com\n\t\tmax_recipients x",
		"non-int max_bytes":    "\t\thost h\n\t\tfrom f@x.com\n\t\tmax_message_bytes x",
		"bad duration":         "\t\thost h\n\t\tfrom f@x.com\n\t\tsend_timeout notaduration",
		"missing arg for host": "",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if name == "missing arg for host" {
				// A bare key with no arg is an arg error.
				if _, err := parse(t, "\t\thost"); err == nil {
					t.Errorf("expected error for %q", name)
				}
				return
			}
			if _, err := parse(t, body); err == nil {
				t.Errorf("expected error for %q", name)
			}
		})
	}
}
