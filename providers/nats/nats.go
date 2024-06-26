package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type DotNats struct {
	ctx  context.Context
	conn *nats.Conn
}

func (d *DotNats) Subscribe(subject string) (<-chan *nats.Msg, error) {
	ch := make(chan *nats.Msg)
	sub, err := d.conn.ChanSubscribe(subject, ch)
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
	return d.conn.Publish(subject, []byte(message))
}

func (d *DotNats) Request(subject, data string, timeout_ ...time.Duration) (*nats.Msg, error) {
	var timeout time.Duration
	switch len(timeout_) {
	case 0:
		timeout = 1 * time.Second
	case 1:
		timeout = timeout_[0]
	default:
		return nil, fmt.Errorf("too many timeout args")
	}

	return d.conn.Request(subject, []byte(data), timeout)
}
