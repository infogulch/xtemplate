package dotsmtp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/infogulch/xtemplate"
	smtpmock "github.com/mocktools/go-smtp-mock/v2"
)

// buildInstance mirrors the dotnats test helper: it builds an xtemplate
// instance backed by an in-memory fs and the given provider options.
func buildInstance(t *testing.T, files map[string]string, opts ...xtemplate.Option) *xtemplate.Instance {
	t.Helper()
	fs := afero.NewMemMapFs()
	for name, content := range files {
		if err := afero.WriteFile(fs, name, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %q to mem fs: %v", name, err)
		}
	}
	cfg := xtemplate.New()
	allOpts := append([]xtemplate.Option{xtemplate.WithTemplateFS(fs)}, opts...)
	inst, _, _, err := cfg.Instance(allOpts...)
	if err != nil {
		t.Fatalf("failed to build instance: %v", err)
	}
	return inst
}

func doRequest(inst *xtemplate.Instance, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, target, nil)
	inst.ServeHTTP(w, r)
	return w
}

// newMockSMTP starts an in-process go-smtp-mock server on a dynamic port and
// tears it down when the test finishes. MultipleMessageReceiving is enabled so
// go-mail's post-delivery RSET preserves the completed message instead of
// flushing it.
func newMockSMTP(t *testing.T) *smtpmock.Server {
	t.Helper()
	srv := smtpmock.New(smtpmock.ConfigurationAttr{
		HostAddress:              "127.0.0.1",
		PortNumber:               0, // OS-assigned
		MultipleMessageReceiving: true,
	})
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start mock smtp server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv
}

// --- Init: defaults & validation (no network) ---

func TestInit_Defaults(t *testing.T) {
	cfg := &DotSMTPConfig{
		Name: "Email",
		Host: "smtp.example.com",
		From: "noreply@example.com",
	}
	if err := cfg.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Defaults are applied on the same struct so they're observable.
	if cfg.Port != 587 {
		t.Errorf("Port = %d, want 587", cfg.Port)
	}
	if cfg.TLS != "starttls" {
		t.Errorf("TLS = %q, want starttls", cfg.TLS)
	}
	if cfg.MaxRecipients != 50 {
		t.Errorf("MaxRecipients = %d, want 50", cfg.MaxRecipients)
	}
	if cfg.MaxMessageBytes != 1<<20 {
		t.Errorf("MaxMessageBytes = %d, want %d", cfg.MaxMessageBytes, 1<<20)
	}
	if cfg.SendTimeout != xtemplate.Duration(30*time.Second) {
		t.Errorf("SendTimeout = %v, want 30s", cfg.SendTimeout)
	}
}

func TestInit_SendTimeoutJSON(t *testing.T) {
	var cfg DotSMTPConfig
	if err := json.Unmarshal([]byte(`{"name":"Email","host":"h","from":"a@b.com","send_timeout":"45s"}`), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg.SendTimeout != xtemplate.Duration(45*time.Second) {
		t.Errorf("SendTimeout = %v, want 45s", cfg.SendTimeout)
	}
}

func TestInit_Validation(t *testing.T) {
	cases := map[string]DotSMTPConfig{
		"missing host": {Name: "Email", From: "noreply@example.com"},
		"missing from": {Name: "Email", Host: "smtp.example.com"},
		"malformed from": {
			Name: "Email", Host: "smtp.example.com", From: "not an address",
		},
		"unknown tls": {
			Name: "Email", Host: "smtp.example.com", From: "noreply@example.com", TLS: "garbage",
		},
		"unknown auth": {
			Name: "Email", Host: "smtp.example.com", From: "noreply@example.com", Auth: "garbage",
		},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if err := cfg.Init(context.Background()); err == nil {
				t.Errorf("expected error for %q, got nil", name)
			}
		})
	}
}

// --- recipient normalisation (no network) ---

func TestNormaliseRecipients(t *testing.T) {
	// Exercise via Send with a config whose client is nil would panic, so we
	// instead drive normalisation through the public Send path on a config
	// whose Init built a client against an unstarted mock — but that requires
	// a server. Instead, assert the error shapes that normalisation produces
	// by calling Send on a DotSMTP with a nil client: normalisation runs
	// before the dial, so its errors surface without a server.
	d := &DotSMTP{cfg: &DotSMTPConfig{
		Host: "x", From: "noreply@example.com",
		MaxRecipients: 50, MaxMessageBytes: 1 << 20,
	}}

	cases := []struct {
		name    string
		to      any
		wantErr string
	}{
		{"nil required", nil, "recipient is required"},
		{"empty string required", "", "recipient is required"},
		{"empty list required", []string{}, "recipient is required"},
		{"empty element", []string{"", "a@x.com"}, "empty address in recipient list"},
		{"non-string element", []any{1}, "must be string"},
		{"wrong type", 42, "must be a string or list of strings"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := d.Send(c.to, "s", "<p>b</p>")
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v, want containing %q", err, c.wantErr)
			}
		})
	}
}

// --- extra-map parsing (no network) ---

func TestExtraMapParsing(t *testing.T) {
	d := &DotSMTP{cfg: &DotSMTPConfig{
		Host: "x", From: "noreply@example.com",
		MaxRecipients: 50, MaxMessageBytes: 1 << 20,
	}}

	t.Run("two maps error", func(t *testing.T) {
		_, err := d.Send("a@x.com", "s", "b",
			map[string]any{"text": "t"}, map[string]any{"text": "t2"})
		if err == nil || !strings.Contains(err.Error(), "at most one extra options map") {
			t.Errorf("err = %v, want at-most-one error", err)
		}
	})
	t.Run("unknown key error", func(t *testing.T) {
		_, err := d.Send("a@x.com", "s", "b", map[string]any{"bogus": 1})
		if err == nil || !strings.Contains(err.Error(), "unknown Send option") {
			t.Errorf("err = %v, want unknown-option error", err)
		}
	})
	t.Run("zero maps ok (passes validation)", func(t *testing.T) {
		_, err := d.Send("a@x.com", "s", "b")
		// Extra parsing and normalisation both passed; the not-initialised
		// guard fires because no Init ran. Assert that guard, not a validation
		// error.
		if err == nil {
			t.Fatal("expected not-initialised error, got nil")
		}
		if !strings.Contains(err.Error(), "not initialized") {
			t.Errorf("err = %v, want not-initialized", err)
		}
	})
}

// --- limit enforcement (no network) ---

func TestLimitEnforcement(t *testing.T) {
	d := &DotSMTP{cfg: &DotSMTPConfig{
		Host: "x", From: "noreply@example.com",
		MaxRecipients: 2, MaxMessageBytes: 10,
	}}

	t.Run("over max_recipients", func(t *testing.T) {
		_, err := d.Send([]string{"a@x.com", "b@x.com", "c@x.com"}, "s", "b")
		if err == nil || !strings.Contains(err.Error(), "exceeds max_recipients") {
			t.Errorf("err = %v, want exceeds max_recipients", err)
		}
	})
	t.Run("at limit ok (passes validation)", func(t *testing.T) {
		_, err := d.Send([]string{"a@x.com", "b@x.com"}, "s", "b")
		if err == nil {
			t.Fatal("expected not-initialised error, got nil")
		}
		if strings.Contains(err.Error(), "max_recipients") {
			t.Errorf("should not hit recipient limit, got %v", err)
		}
	})
	t.Run("over max_message_bytes", func(t *testing.T) {
		_, err := d.Send("a@x.com", "s", "this body is way too long for the limit")
		if err == nil || !strings.Contains(err.Error(), "exceeds max_message_bytes") {
			t.Errorf("err = %v, want exceeds max_message_bytes", err)
		}
	})
}

// --- recipient shape coverage (no network) ---

func TestRecipientShapes(t *testing.T) {
	d := &DotSMTP{cfg: &DotSMTPConfig{
		Host: "x", From: "noreply@example.com",
		MaxRecipients: 50, MaxMessageBytes: 1 << 20,
	}}

	shapes := []struct {
		name string
		to   any
	}{
		{"single string", "alice@example.com"},
		{"display-name string", "Alice <alice@example.com>"},
		{"[]string", []string{"a@x.com", "b@x.com"}},
		{"[]any", []any{"a@x.com", "b@x.com"}},
	}
	for _, c := range shapes {
		t.Run(c.name, func(t *testing.T) {
			_, err := d.Send(c.to, "s", "<p>b</p>")
			// All of these should pass normalisation and limits, then hit the
			// not-initialised guard (no Init ran). Anything mentioning
			// recipient/address is a normalisation regression.
			if err == nil {
				t.Fatal("expected not-initialised error, got nil")
			}
			if strings.Contains(err.Error(), "recipient") || strings.Contains(err.Error(), "address") {
				t.Errorf("normalisation rejected %q: %v", c.name, err)
			}
		})
	}
}

// --- Integration: in-process mock SMTP server ---

func TestDotSMTP_Send_Integration(t *testing.T) {
	srv := newMockSMTP(t)

	cfg := &DotSMTPConfig{
		Name: "Email",
		Host: "127.0.0.1",
		Port: srv.PortNumber(),
		From: "noreply@example.com",
		TLS:  "none",
		Helo: "localhost",
	}
	if err := cfg.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	inst := buildInstance(t,
		map[string]string{
			"send.html": `{{ $id := .Email.Send "test@example.com" "Hello" "<p>World</p>" }}{{ $id }}`,
		},
		xtemplate.WithProvider(cfg),
	)

	w := doRequest(inst, http.MethodGet, "/send")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	msgs, err := srv.WaitForMessagesAndPurge(1, 2*time.Second)
	if err != nil {
		t.Fatalf("waiting for mock message: %v", err)
	}
	// RSET after delivery creates a fresh (empty) message; find the completed
	// one among the recorded sessions.
	var m smtpmock.Message
	for _, cand := range msgs {
		if cand.IsConsistent() {
			m = cand
			break
		}
	}
	if m.MailfromRequest() == "" {
		t.Fatalf("no consistent message among %d recorded; first=%+v", len(msgs), msgs[0])
	}
	if got := m.MailfromRequest(); !strings.Contains(got, "noreply@example.com") {
		t.Errorf("MAIL FROM = %q, want to contain noreply@example.com", got)
	}
	rr := m.RcpttoRequestResponse()
	if len(rr) == 0 || !strings.Contains(rr[0][0], "test@example.com") {
		t.Errorf("RCPT TO = %v, want test@example.com", rr)
	}
	body := m.MsgRequest()
	if !strings.Contains(body, "Subject: Hello") {
		t.Errorf("body missing Subject: Hello\n%s", body)
	}
	if !strings.Contains(body, "World") {
		t.Errorf("body missing rendered HTML content\n%s", body)
	}
	// The template prints the returned Message-ID; html/template escapes the
	// wrapping angle brackets, so the body is &lt;local@host&gt;.
	id := strings.TrimSpace(w.Body.String())
	if !strings.HasPrefix(id, "&lt;") || !strings.HasSuffix(id, "&gt;") || !strings.Contains(id, "@") {
		t.Errorf("returned message id = %q, want an html-escaped <local@host> Message-ID", id)
	}
}
