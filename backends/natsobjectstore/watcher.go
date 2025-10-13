package natsobjectstore

import (
	"context"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Watcher watches a NATS Object Store for changes
type Watcher struct {
	ctx   context.Context
	store jetstream.ObjectStore
}

// NewWatcher creates a new NATS Object Store watcher
func NewWatcher(ctx context.Context, store jetstream.ObjectStore) *Watcher {
	return &Watcher{
		ctx:   ctx,
		store: store,
	}
}

// Start begins watching the NATS Object Store for changes
// This implementation follows the same debouncing pattern as watch.Watch in watch.go
func (w *Watcher) Start(debounce time.Duration, log *slog.Logger, onchange func() bool) (halt chan<- struct{}, err error) {
	log.Debug("starting NATS Object Store watcher", "debounce", debounce)

	// Start watching for updates only (not initial state)
	watcher, err := w.store.Watch(w.ctx, jetstream.UpdatesOnly(), jetstream.IgnoreDeletes())
	if err != nil {
		log.Error("failed to start NATS Object Store watch", "error", err)
		return nil, err
	}

	log.Debug("NATS Object Store watcher created successfully")
	halt_ := make(chan struct{}, 1)

	go func() {
		defer watcher.Stop()

		var timer *time.Timer

		log.Debug("object store watcher goroutine started, waiting for events")

	begin:
		// Wait for first event
		select {
		case info := <-watcher.Updates():
			if info == nil {
				// End of initial data marker
				log.Debug("object store watcher received nil (end of initial data marker)")
				goto begin
			}
			// Got an update, continue to debounce
			log.Debug("object store change detected", "name", info.Name, "size", info.Size)
		case <-halt_:
			goto halt
		case <-w.ctx.Done():
			goto halt
		}

		// Start debounce timer
		timer = time.NewTimer(debounce)
		log.Debug("object store change detected, debouncing", "duration", debounce)

	debounce_loop:
		// Wait for more events or timer expiration
		select {
		case info := <-watcher.Updates():
			if info != nil {
				// Got another update, reset timer
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(debounce)
				goto debounce_loop
			}
			// nil means end of data, shouldn't happen with UpdatesOnly but handle it
			goto debounce_loop
		case <-halt_:
			goto halt
		case <-w.ctx.Done():
			goto halt
		case <-timer.C:
			// Timer expired, trigger reload
		}

		// Call the onchange callback
		if ok := onchange(); !ok {
			goto halt
		}

		// Go back to waiting for next change
		goto begin

	halt:
		log.Debug("object store watcher stopped")
	}()

	return halt_, nil
}

