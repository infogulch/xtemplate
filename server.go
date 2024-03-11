package xtemplate

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

///////////////////////
// Pubic Definitions //
///////////////////////

// Server is a configured, *reloadable*, xtemplate request handler ready to
// execute templates and serve static files in response to http requests. It
// manages an [Instance] and allows you to reload template files with the same
// config by calling `server.Reload()`. If successful, Reload atomically swaps
// the old Instance with the new Instance so subsequent requests are handled by
// the new instance, and any outstanding requests still being served by the old
// Instance can continue to completion. The old instance's Config.Ctx is also
// cancelled.
//
// Create a new Server:
//
// 1. Create a [Config], directly or with [New]
// 2. Configure as desired
// 3. Call [Config.Server]
type Server interface {
	// Handler returns a `http.Handler` that always routes new requests to the
	// current Instance.
	Handler() http.Handler

	// Instance returns the current [Instance]. After calling Reload, this may
	// return a different Instance.
	Instance() Instance

	// Reload creates a new Instance from the config and swaps it with the
	// current instance if successful.
	Reload() error

	// Serve opens a net listener on `listen_addr` and serves requests from it.
	Serve(listen_addr string) error
}

/////////////
// Builder //
/////////////

// Build creates a new xtemplate server instance from an xtemplate.Config.
func (config Config) Server() (Server, error) {
	config.Defaults()
	if config.Logger == nil {
		config.Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(config.LogLevel)}))
	}
	config.Logger = config.Logger.WithGroup("xtemplate")
	if config.Ctx == nil {
		config.Ctx = context.Background()
	}

	server := &xserver{
		config: config,
	}
	err := server.Reload()

	if err != nil {
		return nil, err
	}
	return server, nil
}

////////////////////
// Implementation //
////////////////////

type xserver struct {
	instance atomic.Pointer[xinstance]
	cancel   func()

	mutex  sync.Mutex
	config Config
}

var _ = (Server)((*xserver)(nil))

func (x *xserver) Instance() Instance {
	return x.instance.Load()
}

func (x *xserver) Serve(listen_addr string) error {
	x.config.Logger.Info("starting server")
	return http.ListenAndServe(listen_addr, x.Handler())
}

func (x *xserver) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		x.Instance().ServeHTTP(w, r)
	})
}

func (x *xserver) Reload() error {
	start := time.Now()

	x.mutex.Lock()
	defer x.mutex.Unlock()

	log := x.config.Logger.WithGroup("reload")
	old := x.instance.Load()
	if old != nil {
		log = log.With(slog.Int64("old_id", old.id))
	}

	var newcancel func()
	var new_ *xinstance
	{
		var err error
		config := x.config
		config.Ctx, newcancel = context.WithCancel(x.config.Ctx)
		new_, err = config.instance()
		if err != nil {
			if newcancel != nil {
				newcancel()
			}
			log.Info("failed to load", slog.Any("error", err), slog.Duration("rebuild_time", time.Since(start)))
			return err
		}
	}

	x.instance.CompareAndSwap(old, new_)
	if x.cancel != nil {
		x.cancel()
	}
	x.cancel = newcancel

	log.Info("rebuild succeeded", slog.Int64("new_id", new_.id), slog.Duration("rebuild_time", time.Since(start)))
	return nil
}
