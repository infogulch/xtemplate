package xtemplate

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/infogulch/xtemplate/backends"
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

// Server creates a new Server from an xtemplate.Config.
func (c Config) Server(cfgs ...Option) (*Server, error) {
	if _, err := c.Defaults().Options(cfgs...); err != nil {
		return nil, err
	}

	c.Logger = c.Logger.WithGroup("xtemplate")

	server := &Server{
		config: c,
	}
	err := server.Reload()

	if err != nil {
		// Log the error but don't fail server creation, as it is/might be from no templates being present
		// The watcher will trigger a reload when templates become available
		c.Logger.Warn("initial template load failed, server will retry when templates are available", slog.Any("error", err))
	}

	// Update server's config with backend from the instance if one was created
	// This happens when providers create backends during initialization
	if server.instance.Load() != nil {
		instance := server.instance.Load()
		if instance.config.Backend != nil && server.config.Backend == nil {
			server.config.Backend = instance.config.Backend
		}
	}

	return server, nil
}

// Instance returns the current [Instance]. After calling Reload, previous calls
// to Instance may be stale.
func (x *Server) Instance() *Instance {
	return x.instance.Load()
}

// Backend returns the backend from the server's configuration.
func (x *Server) Backend() backends.Backend {
	return x.config.Backend
}

// Serve opens a net listener on `listen_addr` and serves requests from it.
func (x *Server) Serve(listenAddr string) error {
	x.config.Logger.Info("starting server")
	return http.ListenAndServe(listenAddr, x.Handler())
}

// Handler returns a `http.Handler` that always routes new requests to the
// current Instance. If no instance is loaded yet, returns 503 Service Unavailable.
func (x *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		instance := x.Instance()
		if instance == nil {
			http.Error(w, "Service Unavailable: No templates loaded yet", http.StatusServiceUnavailable)
			return
		}
		instance.ServeHTTP(w, r)
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

		// Update server's config with backend from the instance
		// Providers may create backends during Init(), so we need to propagate it to the server
		// This must happen even if Instance() fails (e.g., empty template store)
		// The instance has its own config copy, so we need to get the backend from it
		if new_ != nil && new_.Backend() != nil && x.config.Backend == nil {
			x.config.Backend = new_.Backend()
			log.Info("updated server backend from instance", "backend", x.config.Backend.Name())
		}

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

func (x *Server) Stop() {
	x.mutex.Lock()
	defer x.mutex.Unlock()

	if x.cancel != nil {
		x.cancel()
	}
	x.cancel = nil
	x.instance.Store(nil)
}
