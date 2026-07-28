package dotnats

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/infogulch/xtemplate"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func init() {
	xtemplate.RegisterProvider("nats", func() xtemplate.Provider { return &DotNatsConfig{} })
}

// WithNats creates an [xtemplate.Option] that adds a nats dot provider to the
// config. Each Instance / Reload build gets a fresh [DotNatsConfig]; options
// pointers are shared via the factory closure.
func WithNats(name string, serverOpts *server.Options, connOpts *nats.Options, jsOpts []jetstream.JetStreamOpt) xtemplate.Option {
	return xtemplate.WithProvider(func() xtemplate.Provider {
		return &DotNatsConfig{Name: name, NatsConfig: &NatsConfig{serverOpts, connOpts, jsOpts}}
	})
}

// NatsConfig holds the configuration needed to connect to a NATS server.
type NatsConfig struct {
	InProcessServerOptions *server.Options          `json:"in_process_server_options"`
	ConnOptions            *nats.Options            `json:"conn_options"`
	JetStreamOptions       []jetstream.JetStreamOpt // encode jetstream opts into json?
}

// DotNatsConfig configures an xtemplate dot field to provide NATS messaging
// access to templates.
type DotNatsConfig struct {
	Name string `json:"name"`

	*NatsConfig `json:"nats_config"`
	Conn        *nats.Conn `json:"-"`

	server *server.Server
	js     jetstream.JetStream

	// owned is true when Init created Conn (and possibly the in-process server).
	// Injected Conn values are not closed.
	owned bool
}

var (
	_ xtemplate.Initializer = &DotNatsConfig{}
	_ xtemplate.Closer      = &DotNatsConfig{}
)

func (d *DotNatsConfig) FieldName() string { return d.Name }
func (d *DotNatsConfig) Prototype() any    { return &DotNats{} }

// Init opens owned connections/servers. Instance context cancel does not destroy
// them; [Close] does after in-flight drain.
func (d *DotNatsConfig) Init(_ context.Context) error {
	var err error
	if d.Conn != nil {
		if d.js == nil {
			var jsOpts []jetstream.JetStreamOpt
			if d.NatsConfig != nil {
				jsOpts = d.JetStreamOptions
			}
			d.js, err = jetstream.New(d.Conn, jsOpts...)
			return err
		}
		return nil
	}
	if d.NatsConfig == nil {
		return fmt.Errorf("no nats client and no config provided to initialize nats client")
	}
	var connOpt nats.Options
	if d.ConnOptions == nil {
		connOpt = nats.GetDefaultOptions()
	} else {
		connOpt = *d.ConnOptions
	}
	if d.InProcessServerOptions != nil {
		// start an internal server for this instance; destroyed only in Close
		// after drain so grace-period handlers keep a live server.
		d.server, err = server.NewServer(d.InProcessServerOptions)
		if err != nil {
			return fmt.Errorf("failed to start in-process nats server: %w", err)
		}
		d.server.Start()

		_ = nats.InProcessServer(d.server)(&connOpt)
	}
	d.Conn, err = connOpt.Connect()
	if err != nil {
		d.cleanupPartial()
		return fmt.Errorf("failed to connect to nats server: %w", err)
	}
	d.owned = true
	d.js, err = jetstream.New(d.Conn, d.JetStreamOptions...)
	if err != nil {
		d.cleanupPartial()
		return err
	}
	return nil
}

// cleanupPartial releases resources opened during a failed Init before Closer
// registration. Safe if nothing was opened yet.
func (d *DotNatsConfig) cleanupPartial() {
	if d.Conn != nil {
		d.Conn.Close()
		d.Conn = nil
	}
	d.owned = false
	d.js = nil
	if d.server != nil {
		d.server.Shutdown()
		d.server = nil
	}
}

// Close drains/closes a connection opened by Init and shuts down any in-process
// server. Injected connections are left alone. Call after in-flight drain so
// grace-period handlers still see a live connection/server.
func (d *DotNatsConfig) Close() error {
	var err error
	if d.owned && d.Conn != nil {
		err = d.Conn.Drain()
		d.Conn = nil
		d.owned = false
	}
	if d.server != nil {
		d.server.Shutdown()
		d.server = nil
	}
	return err
}

func (d *DotNatsConfig) Value(_ http.ResponseWriter, r *http.Request) (any, error) {
	return &DotNats{Conn: d.Conn, JetStream: d.js, ctx: r.Context()}, nil
}

// DotNats provides template access to a NATS connection.
type DotNats struct {
	ctx context.Context

	*nats.Conn
	jetstream.JetStream
}

// Subscribe returns a channel of messages on subject. The channel is closed
// when the request context is cancelled. Delivery uses a buffered callback so
// teardown cannot race with send-on-closed-channel.
func (d *DotNats) Subscribe(subject string) (<-chan *nats.Msg, error) {
	ch := make(chan *nats.Msg, 16)
	var mu sync.Mutex
	open := true
	sub, err := d.Conn.Subscribe(subject, func(m *nats.Msg) {
		mu.Lock()
		defer mu.Unlock()
		if !open {
			return
		}
		select {
		case ch <- m:
		default:
			// drop when consumer is slow (best-effort, like bus fan-out)
		}
	})
	if err != nil {
		return nil, err
	}
	done := d.ctx.Done()
	go func() {
		<-done
		_ = sub.Unsubscribe()
		mu.Lock()
		if open {
			open = false
			close(ch)
		}
		mu.Unlock()
	}()
	return ch, nil
}

func (d *DotNats) Publish(subject, message string) error {
	return d.Conn.Publish(subject, []byte(message))
}

func (d *DotNats) Request(subject, data string, timeout_ ...time.Duration) (*nats.Msg, error) {
	var timeout time.Duration
	switch len(timeout_) {
	case 0:
		timeout = 5 * time.Second
	case 1:
		timeout = timeout_[0]
	default:
		return nil, fmt.Errorf("too many timeout args")
	}

	return d.Conn.Request(subject, []byte(data), timeout)
}
