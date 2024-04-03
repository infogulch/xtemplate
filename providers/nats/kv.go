package nats

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
)

type DotKV struct {
	kv  jetstream.KeyValue
	ctx context.Context
}

func (d *DotKV) Put(key, value string) error {
	_, err := d.kv.PutString(d.ctx, key, value)
	return err
}

func (d *DotKV) Get(key string) (string, error) {
	e, err := d.kv.Get(d.ctx, key)
	if err != nil {
		return "", err
	}
	return string(e.Value()), nil
}

func (d *DotKV) Delete(key string) error {
	return d.kv.Delete(d.ctx, key)
}

func (d *DotKV) Purge(key string) error {
	return d.kv.Purge(d.ctx, key)
}

func (d *DotKV) Watch(keys string) (<-chan jetstream.KeyValueEntry, error) {
	// ctx unsubscribes the watcher on cancel
	watcher, err := d.kv.Watch(d.ctx, keys, jetstream.UpdatesOnly())
	if err != nil {
		return nil, err
	}
	return watcher.Updates(), nil
}
