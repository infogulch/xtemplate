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

type Server interface {
	Instance() Instance
	Serve(listen_addr string) error
	Handler() http.Handler
	Reload() error
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
	x.config.Ctx, newcancel = context.WithCancel(x.config.Ctx)

	var new_ *xinstance
	{
		var err error
		new_, err = x.config.instance()
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
