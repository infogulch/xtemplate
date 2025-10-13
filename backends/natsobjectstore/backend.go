package natsobjectstore

import (
	"context"
	"fmt"

	"github.com/infogulch/xtemplate/backends"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/spf13/afero"
)

// Backend implements the backends.Backend interface for NATS Object Store storage
type Backend struct {
	fs      afero.Fs
	watcher backends.Watcher
}

// Config holds configuration for creating a NATS Object Store backend
// Note: Backend requires an existing NATS connection, JetStream context, or ObjectStore.
// For embedded NATS server creation, use xtemplate.WithNats() provider instead.
type Config struct {
	// Context for operations
	Ctx context.Context

	// NATS connection (if already established)
	// At least one of Conn, JetStream, or ObjectStore must be provided
	Conn *nats.Conn

	// JetStream context (if already established)
	JetStream jetstream.JetStream

	// Object Store (if already established)
	ObjectStore jetstream.ObjectStore

	// JetStream options (used when creating JetStream from Conn)
	JetStreamOptions []jetstream.JetStreamOpt

	// Object Store bucket name (required if ObjectStore not provided)
	Bucket string

	// Optional prefix for all objects
	Prefix string

	// Whether to enable watching for changes
	EnableWatch bool
}

// New creates a new NATS Object Store backend with the given configuration
// At least one of Conn, JetStream, or ObjectStore must be provided in the config.
func New(cfg Config) (*Backend, error) {
	if cfg.Ctx == nil {
		cfg.Ctx = context.Background()
	}

	// Ensure we have an object store
	store := cfg.ObjectStore
	if store == nil {
		// Need to create object store from JetStream
		js := cfg.JetStream
		if js == nil {
			// Need to create JetStream from connection
			if cfg.Conn == nil {
				return nil, fmt.Errorf("at least one of Conn, JetStream, or ObjectStore must be provided")
			}

			// Create JetStream context
			var err error
			js, err = jetstream.New(cfg.Conn, cfg.JetStreamOptions...)
			if err != nil {
				return nil, fmt.Errorf("failed to create JetStream context: %w", err)
			}
		}

		// Create or get object store
		if cfg.Bucket == "" {
			return nil, fmt.Errorf("bucket name is required when ObjectStore is not provided")
		}

		var err error
		store, err = js.CreateOrUpdateObjectStore(cfg.Ctx, jetstream.ObjectStoreConfig{
			Bucket:      cfg.Bucket,
			Description: "xtemplate templates storage",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create object store: %w", err)
		}
	}

	// Create filesystem
	fs := NewFS(cfg.Ctx, store, cfg.Prefix)

	// Create watcher if enabled
	var watcher backends.Watcher
	if cfg.EnableWatch {
		watcher = NewWatcher(cfg.Ctx, store)
	}

	return &Backend{
		fs:      fs,
		watcher: watcher,
	}, nil
}

// FS returns the afero.Fs implementation for this backend
func (b *Backend) FS() afero.Fs {
	return b.fs
}

// Watcher returns the Watcher implementation for this backend
func (b *Backend) Watcher() backends.Watcher {
	return b.watcher
}

// Name returns the name of this backend
func (b *Backend) Name() string {
	return "nats-objstore"
}
