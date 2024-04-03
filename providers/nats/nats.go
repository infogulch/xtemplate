package nats

import (
	"context"

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
