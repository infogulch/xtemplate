package nats

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"

	"github.com/infogulch/xtemplate"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func init() {
	xtemplate.RegisterDot(&DotNatsProvider{})
}

func WithConn(name string, conn *nats.Conn) xtemplate.Option {
	return func(c *xtemplate.Config) error {
		if conn == nil {
			return fmt.Errorf("cannot to create DotNatsProvider with null nats Conn with name %s", name)
		}
		return xtemplate.WithProvider(name, &DotNatsProvider{Conn: conn})(c)
	}
}

func WithConnUrl(name string, url string, opts ...nats.Option) xtemplate.Option {
	return func(c *xtemplate.Config) error {
		return xtemplate.WithProvider(name, &DotNatsProvider{get: func(ctx context.Context) (*nats.Conn, error) {
			conn, err := newConn(nil, url, opts, ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to start connection with name %s: %w", name, err)
			}
			return conn, nil
		}})(c)
	}
}

func WithConnOptions(name string, options *nats.Options) xtemplate.Option {
	return func(c *xtemplate.Config) error {
		if options == nil {
			return fmt.Errorf("cannot to create DotNatsProvider with null nats Options with name %s", name)
		}
		return xtemplate.WithProvider(name, &DotNatsProvider{get: func(ctx context.Context) (*nats.Conn, error) {
			conn, err := newConn(options, "", nil, ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to create connection with name %s: %w", name, err)
			}
			return conn, nil
		}})(c)
	}
}

type DotNatsProvider struct {
	Conn *nats.Conn
	get  func(context.Context) (*nats.Conn, error)
}

var _ encoding.TextUnmarshaler = &DotNatsProvider{}
var _ json.Unmarshaler = &DotNatsProvider{}

func (d *DotNatsProvider) UnmarshalText(b []byte) error {
	url := string(b)
	d.get = func(ctx context.Context) (*nats.Conn, error) {
		return newConn(nil, url, nil, ctx)
	}
	return nil
}
func (d *DotNatsProvider) UnmarshalJSON(b []byte) error {
	var options struct {
		Options *nats.Options `json:"options,omitempty"`
	}
	err := json.Unmarshal(b, &options)
	if err != nil {
		return err
	}
	d.get = func(ctx context.Context) (*nats.Conn, error) {
		conn, err := newConn(options.Options, "", nil, ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create nats connection: %w", err)
		}
		return conn, nil
	}
	return nil
}

var _ xtemplate.DotProvider = &DotNatsProvider{}

func (DotNatsProvider) New() xtemplate.DotProvider { return &DotNatsProvider{} }
func (DotNatsProvider) Type() string               { return "nats" }
func (d *DotNatsProvider) Value(r xtemplate.Request) (any, error) {
	if d.Conn == nil {
		if d.get == nil {
			return &DotNats{}, fmt.Errorf("no nats connection provided")
		}
		conn, err := d.get(r.ServerCtx)
		if err != nil {
			return &DotNats{}, err
		}
		d.Conn = conn
	}
	return &DotNats{conn: d.Conn, ctx: r.R.Context()}, nil
}

func newConn(options *nats.Options, url string, opts []nats.Option, ctx context.Context) (*nats.Conn, error) {
	if options != nil {
		conn, err := options.Connect()
		if err != nil {
			return nil, fmt.Errorf("failed to create connection: %w", err)
		}
		return conn, nil
	} else if url != "" {
		conn, err := nats.Connect(url, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create connection: %w", err)
		}
		return conn, nil
	} else {
		// start an internal server for this instance
		srv, err := server.NewServer(&server.Options{DontListen: true})
		if err != nil {
			return nil, fmt.Errorf("failed to create nats server")
		}
		srv.Start()

		// shut down the server when the xtemplate instance is cancelled
		done := ctx.Done()
		if done != nil {
			go func() {
				<-done
				srv.Shutdown()
			}()
		}

		conn, err := nats.Connect("", append(opts, nats.InProcessServer(srv))...)
		if err != nil {
			return nil, fmt.Errorf("failed to create connection: %w", err)
		}
		return conn, nil
	}
}
