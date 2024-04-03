package nats

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/infogulch/xtemplate"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func init() {
	xtemplate.RegisterDot(&DotKVProvider{})
}

func WithKV(name string, kv jetstream.KeyValue) xtemplate.Option {
	return xtemplate.WithProvider(name, &DotKVProvider{KV: kv})
}

func WithKVUrl(name, bucket, url string, opts ...nats.Option) xtemplate.Option {
	return func(c *xtemplate.Config) error {
		return xtemplate.WithProvider(name, &DotKVProvider{get: func(ctx context.Context) (jetstream.KeyValue, error) {
			return newKV(nil, bucket, nil, nil, nil, url, opts, ctx)
		}})(c)
	}
}

type DotKVProvider struct {
	get func(context.Context) (jetstream.KeyValue, error)
	KV  jetstream.KeyValue
}

var _ encoding.TextUnmarshaler = &DotKVProvider{}
var _ json.Unmarshaler = &DotKVProvider{}

func (d *DotKVProvider) UnmarshalText(b []byte) error {
	parts := strings.Split(string(b), ":")
	if len(parts) != 2 {
		return fmt.Errorf("bad format. flag config must be in the form: URL:BUCKET")
	}
	url, bucket := parts[0], parts[1]
	if bucket == "" {
		return fmt.Errorf("cannot use empty bucket name")
	}
	d.get = func(ctx context.Context) (jetstream.KeyValue, error) {
		return newKV(nil, bucket, nil, nil, nil, url, nil, ctx)
	}
	return nil
}
func (d *DotKVProvider) UnmarshalJSON(b []byte) error {
	var options struct {
		Bucket  string       `json:"bucket"`
		Options nats.Options `json:"options"`
	}
	err := json.Unmarshal(b, &options)
	if err != nil {
		return fmt.Errorf("failed to unmarshal kv options: %w", err)
	}
	if options.Bucket == "" {
		return fmt.Errorf("bucket name must not be empty")
	}
	d.get = func(ctx context.Context) (jetstream.KeyValue, error) {
		return newKV(nil, options.Bucket, nil, nil, &options.Options, "", nil, ctx)
	}
	return nil
}

var _ xtemplate.DotProvider = &DotKVProvider{}

func (DotKVProvider) New() xtemplate.DotProvider { return &DotKVProvider{} }
func (DotKVProvider) Type() string               { return "natskv" }
func (d *DotKVProvider) Value(r xtemplate.Request) (any, error) {
	if d.KV == nil {
		if d.get == nil {
			return &DotKV{}, fmt.Errorf("no kv provided")
		}
		kv, err := d.get(r.ServerCtx)
		if err != nil {
			return &DotKV{}, err
		}
		d.KV = kv
	}
	return &DotKV{d.KV, r.R.Context()}, nil
}

func newKV(kv jetstream.KeyValue, bucket string, js jetstream.JetStream, conn *nats.Conn, options *nats.Options, url string, opts []nats.Option, ctx context.Context) (jetstream.KeyValue, error) {
	var err error
	// I must admit this is a bit strange
	for {
		if kv != nil {
			return kv, nil
		} else if js != nil {
			kv, err = js.KeyValue(ctx, bucket)
			if err != nil {
				return nil, fmt.Errorf("failed to create kv from jetstream: %w", err)
			}
		} else if conn != nil {
			js, err = jetstream.New(conn)
			if err != nil {
				return nil, fmt.Errorf("failed to create jetstream from connection: %w", err)
			}
		} else {
			conn, err = newConn(options, url, opts, ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to create nats connection: %w", err)
			}
		}
	}
}
