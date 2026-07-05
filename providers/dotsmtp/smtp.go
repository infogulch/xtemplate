// Package smtp implements the smtp core dot provider, which exposes
// synchronous send-only SMTP mail delivery to templates via a dot field.
//
// Body rendering stays with the existing .X.Template; this provider only
// transports rendered strings. Sending is synchronous and bounded by
// send_timeout; there is no built-in queue. Applications that want durable
// async delivery should compose this with dotnats/JetStream.
package dotsmtp

import (
	"context"
	"fmt"
	"net/mail"
	"time"

	"github.com/infogulch/xtemplate"
	gomail "github.com/wneessen/go-mail"
)

func init() {
	xtemplate.Register("smtp", func() xtemplate.DotConfig { return &DotSMTPConfig{} })
}

// WithSMTP creates an [xtemplate.Option] that adds an smtp dot provider to the
// config. It is a thin convenience over [xtemplate.WithProvider] for Go-API
// users; Caddyfile/JSON users configure the provider via its struct fields.
func WithSMTP(cfg *DotSMTPConfig) xtemplate.Option {
	return func(c *xtemplate.Config) error {
		c.Providers = append(c.Providers, cfg)
		return nil
	}
}

// DotSMTPConfig configures an xtemplate dot field that provides SMTP mail
// sending to templates.
type DotSMTPConfig struct {
	// Name is the dot field name the provider contributes (required).
	Name string `json:"name"`
	// Host is the SMTP server hostname (required).
	Host string `json:"host"`
	// From is the default RFC 5322 sender address (required; may be overridden
	// per-send via the "from" extra option).
	From string `json:"from"`

	// Port defaults to 587.
	Port int `json:"port"`
	// Username for SMTP auth. When non-empty and Auth is empty, auth defaults
	// to "plain".
	Username string `json:"username"`
	// Password for SMTP auth.
	Password string `json:"password"`
	// Auth selects the SASL mechanism: "plain", "login", "cram-md5", "none",
	// or "" (auto: plain if Username set, else none).
	Auth string `json:"auth"`
	// TLS selects the transport policy: "starttls" (default), "tls" (implicit
	// TLS), or "none".
	TLS string `json:"tls"`
	// Helo is the EHLO/HELO identity sent to the server. When empty, go-mail
	// uses os.Hostname(); set explicitly when the hostname is unsuitable (e.g.
	// inside containers).
	Helo string `json:"helo"`

	// MaxRecipients caps the total number of To+Cc+Bcc recipients per send
	// (default 50).
	MaxRecipients int `json:"max_recipients"`
	// MaxMessageBytes caps len(html)+len(text) per send (default 1 MiB).
	MaxMessageBytes int64 `json:"max_message_bytes"`
	// SendTimeout bounds a single DialAndSend (default 30s).
	SendTimeout time.Duration `json:"send_timeout"`

	// client is built in Init and reused across sends; go-mail dials a fresh
	// connection per DialAndSendWithContext, so the client itself is stateless
	// between sends.
	client *gomail.Client
}

var _ xtemplate.DotConfig = &DotSMTPConfig{}

// FieldName returns the dot field name contributed by this provider.
func (d *DotSMTPConfig) FieldName() string { return d.Name }

// Init applies defaults, validates required fields, and builds the go-mail
// client. It does not dial; go-mail dials per send.
func (d *DotSMTPConfig) Init(ctx context.Context) error {
	if d.Port < 0 {
		return fmt.Errorf("smtp: port must be >= 0")
	}
	if d.MaxMessageBytes < 0 {
		return fmt.Errorf("smtp: max_message_bytes must be >= 0")
	}
	if d.MaxRecipients < 0 {
		return fmt.Errorf("smtp: max_recipients must be >= 0")
	}
	if d.SendTimeout < 0 {
		return fmt.Errorf("smtp: send_timeout must be >= 0")
	}
	if d.Host == "" {
		return fmt.Errorf("smtp: host is required")
	}
	if d.From == "" {
		return fmt.Errorf("smtp: from is required")
	}
	if _, err := mail.ParseAddress(d.From); err != nil {
		return fmt.Errorf("smtp: invalid default from address %q: %w", d.From, err)
	}

	// Apply defaults.
	if d.Port == 0 {
		d.Port = 587
	}
	if d.TLS == "" {
		d.TLS = "starttls"
	}
	if d.MaxRecipients == 0 {
		d.MaxRecipients = 50
	}
	if d.MaxMessageBytes == 0 {
		d.MaxMessageBytes = 1 << 20
	}
	if d.SendTimeout == 0 {
		d.SendTimeout = 30 * time.Second
	}

	opts := []gomail.Option{gomail.WithPort(d.Port)}

	switch d.TLS {
	case "tls":
		// Implicit TLS (typically port 465).
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSMandatory), gomail.WithSSL())
	case "starttls":
		// Explicit STARTTLS (typically port 587).
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSMandatory))
	case "none":
		opts = append(opts, gomail.WithTLSPolicy(gomail.NoTLS))
	default:
		return fmt.Errorf("smtp: unknown tls policy %q (want tls, starttls, or none)", d.TLS)
	}

	authType := d.Auth
	if authType == "" {
		if d.Username != "" {
			authType = "plain"
		} else {
			authType = "none"
		}
	}
	switch authType {
	case "plain":
		opts = append(opts, gomail.WithSMTPAuth(gomail.SMTPAuthPlain))
	case "login":
		opts = append(opts, gomail.WithSMTPAuth(gomail.SMTPAuthLogin))
	case "cram-md5":
		opts = append(opts, gomail.WithSMTPAuth(gomail.SMTPAuthCramMD5))
	case "none":
		// no auth
	default:
		return fmt.Errorf("smtp: unknown auth type %q (want plain, login, cram-md5, or none)", d.Auth)
	}

	if d.Username != "" {
		opts = append(opts, gomail.WithUsername(d.Username))
	}
	if d.Password != "" {
		opts = append(opts, gomail.WithPassword(d.Password))
	}
	if d.Helo != "" {
		opts = append(opts, gomail.WithHELO(d.Helo))
	}
	opts = append(opts, gomail.WithTimeout(d.SendTimeout))

	client, err := gomail.NewClient(d.Host, opts...)
	if err != nil {
		return fmt.Errorf("smtp: build client: %w", err)
	}
	d.client = client
	return nil
}

// Value returns the per-request dot value. The returned type carries the
// request context so a cancelled request aborts the in-flight send.
func (d *DotSMTPConfig) Value(r xtemplate.Request) (any, error) {
	return &DotSMTP{cfg: d, ctx: r.R.Context()}, nil
}

// DotSMTP is the per-request dot value exposed to templates. Call Send to
// deliver a message synchronously.
type DotSMTP struct {
	cfg *DotSMTPConfig
	ctx context.Context
}

// Send delivers a single message synchronously and returns the generated
// Message-ID.
//
//	to      any              – a single address string or a []string / []any of
//	                          address strings. Each string is one RFC 5322
//	                          address (display-name form allowed); recipients
//	                          are not split on commas. Required.
//	subject string           – the Subject header.
//	body    string           – the HTML body.
//	extra   ...map[string]any – zero or one options map. Supported keys:
//	                          "cc", "bcc" (string or list, as per "to"),
//	                          "from" (string override), "replyTo" (string),
//	                          "text" (plaintext alternative). Unknown keys
//	                          are rejected.
func (d *DotSMTP) Send(to any, subject, body string, extra ...map[string]any) (string, error) {
	if len(extra) > 1 {
		return "", fmt.Errorf("smtp: Send accepts at most one extra options map, got %d", len(extra))
	}

	msg := map[string]any{
		"to":      to,
		"subject": subject,
		"html":    body,
	}
	if len(extra) == 1 {
		for k, v := range extra[0] {
			switch k {
			case "cc", "bcc", "from", "replyTo", "text":
				msg[k] = v
			default:
				return "", fmt.Errorf("smtp: unknown Send option %q", k)
			}
		}
	}
	return d.sendMap(msg)
}

// sendMap is the single source of truth for assembling and delivering a
// message from the canonical schema. A future SendMap entry point will call
// this directly.
//
//	Canonical schema:
//	  "to"      any        string or []string (normalised)
//	  "subject" string
//	  "html"    string
//	  "text"    string     (optional)
//	  "cc"      []string   (optional)
//	  "bcc"     []string   (optional)
//	  "from"    string     (optional; defaults to cfg.From)
//	  "replyTo" string     (optional)
func (d *DotSMTP) sendMap(m map[string]any) (string, error) {
	to, err := normaliseRecipients(m["to"], true)
	if err != nil {
		return "", err
	}
	cc, err := normaliseRecipients(m["cc"], false)
	if err != nil {
		return "", err
	}
	bcc, err := normaliseRecipients(m["bcc"], false)
	if err != nil {
		return "", err
	}

	getField := func(key string) (string, error) {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok {
				return s, nil
			} else {
				return "", fmt.Errorf("smtp: %s field must be a string, got %T", key, v)
			}
		}
		return "", nil
	}

	subject, err := getField("subject")
	if err != nil {
		return "", err
	}
	html, err := getField("html")
	if err != nil {
		return "", err
	}
	text, err := getField("text")
	if err != nil {
		return "", err
	}

	// Recipient limit.
	if total := len(to) + len(cc) + len(bcc); total > d.cfg.MaxRecipients {
		return "", fmt.Errorf("smtp: %d recipients exceeds max_recipients %d", total, d.cfg.MaxRecipients)
	}
	// Conservative body-size pre-check (the assembled MIME message is slightly
	// larger due to headers/encoding).
	if bodyBytes := int64(len(html) + len(text)); bodyBytes > d.cfg.MaxMessageBytes {
		return "", fmt.Errorf("smtp: body of %d bytes exceeds max_message_bytes %d", bodyBytes, d.cfg.MaxMessageBytes)
	}
	if d.cfg.client == nil {
		return "", fmt.Errorf("smtp: client not initialized; Init was not called or failed")
	}

	from, err := getField("from")
	if err != nil {
		return "", err
	} else if from == "" {
		from = d.cfg.From
	} else if _, err := mail.ParseAddress(from); err != nil {
		return "", fmt.Errorf("smtp: invalid from address %q: %w", from, err)
	}

	replyTo, err := getField("replyTo")
	if err != nil {
		return "", err
	} else if replyTo != "" {
		if _, err := mail.ParseAddress(replyTo); err != nil {
			return "", fmt.Errorf("smtp: invalid reply-to address %q: %w", replyTo, err)
		}
	}

	gm := gomail.NewMsg()
	if err := gm.From(from); err != nil {
		return "", fmt.Errorf("smtp: set from: %w", err)
	}
	if err := gm.To(to...); err != nil {
		return "", fmt.Errorf("smtp: set to: %w", err)
	}
	if len(cc) > 0 {
		if err := gm.Cc(cc...); err != nil {
			return "", fmt.Errorf("smtp: set cc: %w", err)
		}
	}
	if len(bcc) > 0 {
		if err := gm.Bcc(bcc...); err != nil {
			return "", fmt.Errorf("smtp: set bcc: %w", err)
		}
	}
	if replyTo != "" {
		if err := gm.ReplyTo(replyTo); err != nil {
			return "", fmt.Errorf("smtp: set reply-to: %w", err)
		}
	}
	gm.Subject(subject)

	switch {
	case html != "" && text != "":
		gm.SetBodyString(gomail.TypeTextHTML, html)
		gm.AddAlternativeString(gomail.TypeTextPlain, text)
	case html != "":
		gm.SetBodyString(gomail.TypeTextHTML, html)
	case text != "":
		gm.SetBodyString(gomail.TypeTextPlain, text)
	default:
		gm.SetBodyString(gomail.TypeTextPlain, "")
	}

	// Generate a Message-ID so callers can correlate the sent message.
	gm.SetMessageID()

	if err := d.cfg.client.DialAndSendWithContext(d.ctx, gm); err != nil {
		return "", fmt.Errorf("smtp: send: %w", err)
	}
	return gm.GetMessageID(), nil
}

// normaliseRecipients parses a recipient field into a list of validated RFC
// 5322 address strings. One input string is exactly one address — no
// comma-splitting. When required is false, an empty/nil field is allowed and
// yields nil (the caller omits the header).
func normaliseRecipients(v any, required bool) ([]string, error) {
	switch val := v.(type) {
	case nil:
		if required {
			return nil, fmt.Errorf("smtp: recipient is required")
		}
		return nil, nil
	case string:
		if val == "" {
			if required {
				return nil, fmt.Errorf("smtp: recipient is required")
			}
			return nil, nil
		}
		addr, err := mail.ParseAddress(val)
		if err != nil {
			return nil, fmt.Errorf("smtp: invalid address %q: %w", val, err)
		}
		return []string{addr.String()}, nil
	case []string:
		if len(val) == 0 {
			if required {
				return nil, fmt.Errorf("smtp: recipient is required")
			}
			return nil, nil
		}
		out := make([]string, 0, len(val))
		for _, s := range val {
			if s == "" {
				return nil, fmt.Errorf("smtp: empty address in recipient list")
			}
			addr, err := mail.ParseAddress(s)
			if err != nil {
				return nil, fmt.Errorf("smtp: invalid address %q: %w", s, err)
			}
			out = append(out, addr.String())
		}
		return out, nil
	case []any:
		if len(val) == 0 {
			if required {
				return nil, fmt.Errorf("smtp: recipient is required")
			}
			return nil, nil
		}
		out := make([]string, 0, len(val))
		for _, e := range val {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("smtp: recipient list element must be string, got %T", e)
			}
			if s == "" {
				return nil, fmt.Errorf("smtp: empty address in recipient list")
			}
			addr, err := mail.ParseAddress(s)
			if err != nil {
				return nil, fmt.Errorf("smtp: invalid address %q: %w", s, err)
			}
			out = append(out, addr.String())
		}
		return out, nil
	default:
		return nil, fmt.Errorf("smtp: recipient must be a string or list of strings, got %T", v)
	}
}
