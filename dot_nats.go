package xtemplate

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type DotNats struct {
	ctx context.Context

	*nats.Conn
	jetstream.JetStream
}

func (d *DotNats) Subscribe(subject string) (<-chan *nats.Msg, error) {
	ch := make(chan *nats.Msg)
	sub, err := d.Conn.ChanSubscribe(subject, ch)
	if err != nil {
		return nil, err
	}
	done := d.ctx.Done()
	go func() {
		<-done
		sub.Unsubscribe()
		close(ch)
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
