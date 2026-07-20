// Package dotbus implements the bus core dot provider: a process-local
// multi-producer multi-consumer topic fan-out for templates.
//
// Use it for single-process SSE and live UI messaging. Prefer the nats
// provider when you need multi-process delivery, request/reply, persistence,
// or JetStream.
package dotbus

import (
	"context"
	"fmt"

	"github.com/infogulch/xtemplate"
)

func init() {
	xtemplate.Register("bus", func() xtemplate.DotConfig { return &DotBusConfig{} })
}

// WithBus creates an [xtemplate.Option] that adds a bus dot provider.
// buffer is the per-subscriber channel capacity; 0 means [DefaultBuffer].
func WithBus(name string, buffer int) xtemplate.Option {
	return func(c *xtemplate.Config) error {
		c.Providers = append(c.Providers, &DotBusConfig{Name: name, Buffer: buffer})
		return nil
	}
}

// DotBusConfig configures an xtemplate dot field for in-process topic fan-out.
type DotBusConfig struct {
	// Name is the dot field name (required), e.g. "Bus" → {{.Bus}}.
	Name string `json:"name"`
	// Buffer is the per-subscriber channel capacity. 0 means DefaultBuffer.
	Buffer int `json:"buffer"`

	bus *Bus
}

var _ xtemplate.DotConfig = &DotBusConfig{}

// FieldName returns the dot field name contributed by this provider.
func (d *DotBusConfig) FieldName() string { return d.Name }

// Init validates config, creates the bus, and shuts it down when ctx is cancelled
// (instance reload / server stop).
func (d *DotBusConfig) Init(ctx context.Context) error {
	if d.Name == "" {
		return fmt.Errorf("bus: name is required")
	}
	if d.Buffer < 0 {
		return fmt.Errorf("bus: buffer must be >= 0")
	}
	d.bus = New(d.Buffer)
	if done := ctx.Done(); done != nil {
		go func() {
			<-done
			d.bus.Shutdown()
		}()
	}
	return nil
}

// Value returns the per-request [DotBus] bound to the request context.
func (d *DotBusConfig) Value(r xtemplate.Request) (any, error) {
	return &DotBus{bus: d.bus, ctx: r.R.Context()}, nil
}

// DotBus is the per-request template value for the bus provider.
type DotBus struct {
	bus *Bus
	ctx context.Context
}

// Publish sends message to all current subscribers of topic. Slow subscribers
// may drop the message; Publish never blocks on them.
func (d *DotBus) Publish(topic, message string) error {
	return d.bus.Publish(topic, message)
}

// Subscribe returns a channel of messages on topic. The channel is closed when
// the request context is cancelled or the bus shuts down, so
// {{range .Bus.Subscribe "topic"}} ends cleanly for SSE handlers.
func (d *DotBus) Subscribe(topic string) (<-chan string, error) {
	ch, err := d.bus.Subscribe(topic)
	if err != nil {
		return nil, err
	}
	if done := d.ctx.Done(); done != nil {
		go func() {
			<-done
			d.bus.Unsubscribe(ch)
		}()
	}
	return ch, nil
}
