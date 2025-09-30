package xtemplate

import (
	"context"
	"fmt"

	"github.com/infogulch/xtemplate/backends/natsobjectstore"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// NatsBackendConfig holds configuration for creating a NATS Object Store backend
// from a NATS provider. This allows the provider to create and manage both the
// NATS server infrastructure and the backend storage.
type NatsBackendConfig struct {
	// Object Store bucket name
	Bucket string

	// Optional prefix for all objects
	Prefix string

	// Whether to enable watching for changes (hot reload)
	EnableWatch bool
}

// WithNats creates a NATS provider that manages NATS server infrastructure.
// If backendCfg is provided, the provider will also create a NATS Object Store backend.
func WithNats(name string, serverOpts *server.Options, connOpts *nats.Options, jsOpts []jetstream.JetStreamOpt, backendCfg *NatsBackendConfig) Option {
	return func(c *Config) error {
		c.Nats = append(c.Nats, &DotNatsConfig{
			Name:          name,
			NatsConfig:    &NatsConfig{serverOpts, connOpts, jsOpts},
			BackendConfig: backendCfg,
		})
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

	*NatsConfig   `json:"nats_config"`
	BackendConfig *NatsBackendConfig `json:"backend_config,omitempty"`

	Conn   *nats.Conn
	server *server.Server
	js     jetstream.JetStream
}

var _ DotConfig = &DotNatsConfig{}

func (d *DotNatsConfig) FieldName() string { return d.Name }

func (d *DotNatsConfig) Init(ctx context.Context, config *Config) error {
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

		// Note: We do NOT shut down the server when the instance context is cancelled
		// because we want to reuse the same NATS server across hot reloads.
		// The server will be shut down when the application exits.
		// If you need to shut down the server, call d.server.Shutdown() explicitly.

		// Use in-process connection for this client
		// The server will still listen on TCP (unless DontListen is set) for external clients
		nats.InProcessServer(d.server)(&connOpt)
	}
	d.Conn, err = connOpt.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	d.js, err = jetstream.New(d.Conn, d.NatsConfig.JetStreamOptions...)
	if err != nil {
		return err
	}

	// Create backend if configured and not already set
	if d.BackendConfig != nil && config.Backend == nil {
		backend, err := natsobjectstore.New(natsobjectstore.Config{
			Ctx:         ctx,
			Conn:        d.Conn,
			JetStream:   d.js,
			Bucket:      d.BackendConfig.Bucket,
			Prefix:      d.BackendConfig.Prefix,
			EnableWatch: d.BackendConfig.EnableWatch,
		})
		if err != nil {
			return fmt.Errorf("failed to create NATS backend: %w", err)
		}
		config.Backend = backend
	}

	return nil
}

func (d *DotNatsConfig) Value(r Request) (any, error) {
	return &DotNats{Conn: d.Conn, JetStream: d.js, ctx: r.R.Context()}, nil
}
