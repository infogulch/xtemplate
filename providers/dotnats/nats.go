package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/infogulch/xtemplate"
	"github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func init() {
	xtemplate.Register("nats", func() xtemplate.DotConfig { return &DotNatsConfig{} })
}

// WithNats creates an [xtemplate.Option] that adds a nats dot provider to the
// config.
func WithNats(name string, serverOpts *server.Options, connOpts *natsgo.Options, jsOpts []jetstream.JetStreamOpt) xtemplate.Option {
	return func(c *xtemplate.Config) error {
		c.Providers = append(c.Providers, &DotNatsConfig{Name: name, NatsConfig: &NatsConfig{serverOpts, connOpts, jsOpts}})
		return nil
	}
}

// NatsConfig holds the configuration needed to connect to a NATS server.
type NatsConfig struct {
	InProcessServerOptions *server.Options          `json:"in_process_server_options"`
	ConnOptions            *natsgo.Options          `json:"conn_options"`
	JetStreamOptions       []jetstream.JetStreamOpt // encode jetstream opts into json?
}

// DotNatsConfig configures an xtemplate dot field to provide NATS messaging
// access to templates.
type DotNatsConfig struct {
	Name string `json:"name"`

	*NatsConfig `json:"nats_config"`
	Conn        *natsgo.Conn `json:"-"`

	server *server.Server
	js     jetstream.JetStream
}

var _ xtemplate.DotConfig = &DotNatsConfig{}

func (d *DotNatsConfig) FieldName() string { return d.Name }

func (d *DotNatsConfig) Init(ctx context.Context) error {
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
	var connOpt natsgo.Options
	if d.ConnOptions == nil {
		connOpt = natsgo.GetDefaultOptions()
	} else {
		connOpt = *d.ConnOptions
	}
	if d.InProcessServerOptions != nil {
		// start an internal server for this instance
		d.server, err = server.NewServer(d.InProcessServerOptions)
		if err != nil {
			return fmt.Errorf("failed to start in-process nats server: %w", err)
		}
		d.server.Start()

		// shut down the server when the instance is cancelled
		done := ctx.Done()
		if done != nil {
			go func() {
				<-done
				d.server.Shutdown()
			}()
		}

		_ = natsgo.InProcessServer(d.server)(&connOpt)
	}
	d.Conn, err = connOpt.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to in-process server: %w", err)
	}
	d.js, err = jetstream.New(d.Conn, d.JetStreamOptions...)
	return err
}

func (d *DotNatsConfig) Value(r xtemplate.Request) (any, error) {
	return &DotNats{Conn: d.Conn, JetStream: d.js, ctx: r.R.Context()}, nil
}

// DotNats provides template access to a NATS connection.
type DotNats struct {
	ctx context.Context

	*natsgo.Conn
	jetstream.JetStream
}

func (d *DotNats) Subscribe(subject string) (<-chan *natsgo.Msg, error) {
	ch := make(chan *natsgo.Msg)
	sub, err := d.ChanSubscribe(subject, ch)
	if err != nil {
		return nil, err
	}
	done := d.ctx.Done()
	go func() {
		<-done
		_ = sub.Unsubscribe()
		close(ch)
	}()
	return ch, nil
}

func (d *DotNats) Publish(subject, message string) error {
	return d.Conn.Publish(subject, []byte(message))
}

func (d *DotNats) Request(subject, data string, timeout_ ...time.Duration) (*natsgo.Msg, error) {
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
