package xtemplate

import (
	"context"
	"fmt"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func WithNats(name string, serverOpts *server.Options, connOpts *nats.Options, jsOpts []jetstream.JetStreamOpt) Option {
	return func(c *Config) error {
		c.Nats = append(c.Nats, DotNatsConfig{Name: name, NatsConfig: &NatsConfig{serverOpts, connOpts, jsOpts}})
		return nil
	}
}

type NatsConfig struct {
	InProcessServerOptions *server.Options          `json:"in_process_server_options"`
	ConnOptions            *nats.Options            `json:"conn_options"`
	JetStreamOptions       []jetstream.JetStreamOpt // encode jetstream opts into json?
}

type DotNatsConfig struct {
	Name string `json:"name"`

	*NatsConfig `json:"nats_config"`
	Conn        *nats.Conn

	server *server.Server
	js     jetstream.JetStream
}

var _ DotConfig = &DotNatsConfig{}

func (d *DotNatsConfig) FieldName() string { return d.Name }
func (d *DotNatsConfig) Init(ctx context.Context) error {
	var err error
	if d.Conn != nil {
		if d.js == nil {
			var jsOpts []jetstream.JetStreamOpt
			if d.NatsConfig != nil {
				jsOpts = d.NatsConfig.JetStreamOptions
			}
			d.js, err = jetstream.New(d.Conn, jsOpts...)
			return err
		}
		return nil
	}
	if d.NatsConfig == nil {
		return fmt.Errorf("no nats client and no config provided to initialzie nats client")
	}
	var connOpt nats.Options
	if d.NatsConfig.ConnOptions == nil {
		connOpt = nats.GetDefaultOptions()
	} else {
		connOpt = *d.NatsConfig.ConnOptions
	}
	if d.NatsConfig.InProcessServerOptions != nil {
		// start an internal server for this instance
		d.server, err = server.NewServer(d.NatsConfig.InProcessServerOptions)
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

		nats.InProcessServer(d.server)(&connOpt)
	}
	d.Conn, err = connOpt.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to in-process server: %w", err)
	}
	d.js, err = jetstream.New(d.Conn, d.NatsConfig.JetStreamOptions...)
	return err
}
func (d *DotNatsConfig) Value(r Request) (any, error) {
	return &DotNats{Conn: d.Conn, JetStream: d.js, ctx: r.R.Context()}, nil
}
