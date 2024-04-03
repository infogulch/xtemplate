package xtemplate

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Server is a configured, *reloadable*, xtemplate request handler ready to
// execute templates and serve static files in response to http requests. It
// manages an [Instance] and allows you to reload template files with the same
// config by calling `server.Reload()`. If successful, Reload atomically swaps
// the old Instance with the new Instance so subsequent requests are handled by
// the new instance, and any outstanding requests still being served by the old
// Instance can continue to completion. The old instance's Config.Ctx is also
// cancelled.
//
// The only way to create a valid *Server is to call [Config.Server].
type Server struct {
	instance atomic.Pointer[Instance]
	cancel   func()

	mutex  sync.Mutex
	config Config
}

// Build creates a new Server from an xtemplate.Config.
func (config Config) Server(cfgs ...Option) (*Server, error) {
	config.Defaults()
	for _, c := range cfgs {
		if err := c(&config); err != nil {
			return nil, fmt.Errorf("failed to configure server: %w", err)
		}
	}

	config.Logger = config.Logger.WithGroup("xtemplate")

	server := &Server{
		config: config,
	}
	err := server.Reload()

	if err != nil {
		return nil, err
	}
	return server, nil
}

// Instance returns the current [Instance]. After calling Reload, previous calls
// to Instance may be stale.
func (x *Server) Instance() *Instance {
	return x.instance.Load()
}

// Serve opens a net listener on `listen_addr` and serves requests from it.
func (x *Server) Serve(listen_addr string) error {
	x.config.Logger.Info("starting server")
	return http.ListenAndServe(listen_addr, x.Handler())
}

// Handler returns a `http.Handler` that always routes new requests to the
// current Instance.
func (x *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		x.Instance().ServeHTTP(w, r)
	})
}

// Reload creates a new Instance from the config and swaps it with the
// current instance if successful, otherwise returns the error.
func (x *Server) Reload(cfgs ...Option) error {
	start := time.Now()

	x.mutex.Lock()
	defer x.mutex.Unlock()

	log := x.config.Logger.WithGroup("reload")
	old := x.instance.Load()
	if old != nil {
		log = log.With(slog.Int64("old_id", old.id))
	}

	var newcancel func()
	var new_ *Instance
	{
		var err error
		config := x.config
		config.Ctx, newcancel = context.WithCancel(x.config.Ctx)
		new_, _, _, err = config.Instance(cfgs...)
		if err != nil {
			newcancel()
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
