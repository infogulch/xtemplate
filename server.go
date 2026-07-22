package xtemplate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// defaultGrace is the bound for draining in-flight work on Serve cancel and
// on instance retire after Reload. Not a fixed sleep: wait returns early when idle.
const defaultGrace = 5 * time.Second

// Server is a configured, *reloadable*, xtemplate request handler ready to
// execute templates and serve static files in response to http requests. It
// implements [http.Handler] by always routing to the current [Instance].
//
// Call [Server.Reload] to rebuild from the same config (or with options). If
// successful, Reload atomically swaps the old Instance for the new one so
// subsequent requests use the new instance; outstanding requests on the old
// Instance are given a grace period to finish after the old instance context
// is cancelled, then providers are closed.
//
// When [TemplateSource.Start] returned a nil initial FS, Reload options must
// include [WithTemplateFS] or [WithTemplateDir]: the sticky base FS is only a
// 503 placeholder and reload options are not written back onto the base.
//
// Call [Server.Shutdown] for a graceful stop or [Server.Stop] for immediate
// teardown. When using [Server.Serve], cancelling the server context also
// drains the local [http.Server] (Serve owns that; Server does not store it).
//
// The only way to create a valid *Server is to call [Config.Server].
type Server struct {
	instance atomic.Pointer[Instance]
	cancel   context.CancelFunc // cancels current instance ctx

	mutex  sync.Mutex
	config Config

	serverCtx    context.Context
	serverCancel context.CancelFunc
}

var _ http.Handler = (*Server)(nil)

// Server creates a new Server from an xtemplate.Config.
func (config Config) Server(options ...Option) (*Server, error) {
	if _, err := config.SetDefaults().Options(options...); err != nil {
		return nil, err
	}

	config.Logger = config.Logger.WithGroup("xtemplate")

	if err := config.resolveSourceRaw(); err != nil {
		return nil, err
	}
	config.ensureSource()

	serverCtx, serverCancel := context.WithCancel(config.Ctx)

	server := &Server{
		config:       config,
		serverCtx:    serverCtx,
		serverCancel: serverCancel,
	}

	initial, reloads, err := config.Source.Start(serverCtx, config.Logger.WithGroup("source"))
	if err != nil {
		serverCancel()
		return nil, err
	}
	if initial != nil {
		server.config.templatesFS = initial
	} else if reloads != nil {
		server.config.templatesFS = notReadyTemplatesFS
	} else {
		serverCancel()
		return nil, fmt.Errorf("xtemplate: source failed to provide initial fs or reload channel")
	}

	// First instance Ctx is a child of serverCtx so Stop/Shutdown cancel SSE
	// without requiring the parent Config.Ctx to be cancelled.
	var instanceCancel context.CancelFunc
	server.config.Ctx, instanceCancel = context.WithCancel(serverCtx)
	server.cancel = instanceCancel

	new_, _, _, err := server.config.buildInstance()
	if err != nil {
		instanceCancel()
		serverCancel()
		return nil, err
	}
	server.instance.Store(new_)

	// Do not start the reload consumer until the first instance build succeeds.
	if reloads != nil {
		go func() {
			log := server.config.Logger.WithGroup("reload")
			for {
				select {
				case <-server.serverCtx.Done():
					return
				case opts, ok := <-reloads:
					if !ok {
						return
					}
					if err := server.Reload(opts...); err != nil {
						log.Error("reload failed", slog.Any("error", err))
					}
				}
			}
		}()
	}

	return server, nil
}

// Reload creates a new Instance from the config and swaps it with the
// current instance if successful, otherwise returns the error. The previous
// instance context is cancelled, in-flight requests are waited on up to
// [defaultGrace], then providers are closed.
//
// WithSource is rejected. WithTemplateFS/Dir update the copy's private FS
// for this build only (not sticky on the Server base). Empty opts rebuild
// from the base templatesFS (from Source.Start).
//
// If Source.Start returned a nil initial FS, opts must include WithTemplateFS
// or WithTemplateDir so a failed or FS-less reload cannot replace live content
// with the 503 placeholder.
func (x *Server) Reload(options ...Option) (err error) {
	start := time.Now()

	x.mutex.Lock()

	if ctxErr := x.serverCtx.Err(); ctxErr != nil {
		x.mutex.Unlock()
		// Options were never applied — free per-build resources (e.g. git clone dirs).
		_ = invokeOnCloseFromOptions(options)
		err := errors.New("server stopped")
		if notify := reloadResultFromOptions(options); notify != nil {
			notify(err)
		}
		return err
	}

	log := x.config.Logger.WithGroup("reload")
	old := x.instance.Load()
	if old != nil {
		log = log.With(slog.Int64("old_id", old.id))
	}

	// Capture notify before build: buildInstance returns nil Instance on error.
	notify := reloadResultFromOptions(options)

	var newcancel context.CancelFunc
	var new_ *Instance
	{
		config := x.config
		config.Ctx, newcancel = context.WithCancel(x.serverCtx)
		new_, _, _, err = config.buildInstance(options...)
		if err != nil {
			newcancel()
			x.mutex.Unlock()
			// buildInstance already ran OnClose via closeOnce when Options were applied.
			if notify != nil {
				notify(err)
			}
			log.Info("failed to load", slog.Any("error", err), slog.Duration("rebuild_time", time.Since(start)))
			return err
		}
		if new_.config.templatesFS == notReadyTemplatesFS {
			newcancel()
			closeErr := new_.Close()
			x.mutex.Unlock()
			err = errors.New("xtemplate: reload must include WithTemplateFS or WithTemplateDir when Source.Start returned nil initial FS")
			if notify != nil {
				notify(err)
			}
			log.Info("failed to load", slog.Any("error", err), slog.Duration("rebuild_time", time.Since(start)), slog.Any("close_error", closeErr))
			return err
		}
	}

	old = x.instance.Swap(new_)
	oldCancel := x.cancel
	x.cancel = newcancel
	x.mutex.Unlock()

	if notify != nil {
		notify(nil)
	}

	if old != nil {
		graceCtx, graceCancel := context.WithTimeout(context.Background(), defaultGrace)
		x.retire(old, oldCancel, graceCtx)
		graceCancel()
	}

	log.Info("rebuild succeeded", slog.Int64("new_id", new_.id), slog.Duration("rebuild_time", time.Since(start)))
	return nil
}

// Instance returns the current [Instance]. After calling Reload, previous calls
// to Instance may be stale. After Stop/Shutdown, returns nil.
func (x *Server) Instance() *Instance {
	return x.instance.Load()
}

// Serve opens a net listener on `listen_addr` and serves requests from it.
// It returns when the listener fails or when the server context is cancelled
// (parent [Config.Ctx] or [Server.Shutdown]/[Server.Stop]), in which case the
// local [http.Server] is drained (default grace [defaultGrace]), the instance
// is retired, and Serve returns nil.
func (x *Server) Serve(listen_addr string) error {
	ln, err := net.Listen("tcp", listen_addr)
	if err != nil {
		return err
	}
	// Log the actual bound address (resolved from listen_addr) so the port is
	// visible in the logs, including when listen_addr requests an ephemeral
	// port like ":0".
	x.config.Logger.Info("starting server", slog.String("address", ln.Addr().String()))

	srv := &http.Server{
		Handler:           x,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		<-x.serverCtx.Done()
		drainCtx, cancel := context.WithTimeout(context.Background(), defaultGrace)
		defer cancel()
		// Retire instance first (serverCtx already cancelled → SSE unblocks),
		// then drain this Serve-local http.Server. Server does not own *http.Server.
		_ = x.Shutdown(drainCtx)
		_ = srv.Shutdown(drainCtx)
	}()

	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// ServeHTTP routes the request to the current [Instance], or responds 503 if
// the server has been stopped.
func (x *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	instance := x.Instance()
	if instance == nil {
		http.Error(w, "server stopped", http.StatusServiceUnavailable)
		return
	}
	instance.ServeHTTP(w, r)
}

// Shutdown stops the server gracefully.
//
//  1. Nils the current instance (new requests get 503) and cancels serverCtx
//     (cascades into the instance context so SSE/Flush observe stop).
//  2. Waits for in-flight instance requests up to ctx, then Closes providers.
//
// When [Server.Serve] is running, cancelling serverCtx also causes Serve to
// drain its local [http.Server]; Shutdown itself does not own or call into it.
//
// ctx bounds only the in-flight wait; teardown always runs. Safe if Serve never
// ran (handler-only / Caddy). Idempotent.
func (x *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	x.mutex.Lock()
	old := x.instance.Swap(nil)
	oldCancel := x.cancel
	x.cancel = nil

	if x.serverCancel != nil {
		x.serverCancel()
	}
	x.mutex.Unlock()

	// Cancel instance explicitly as well (no-op if already cancelled via serverCtx).
	if oldCancel != nil {
		oldCancel()
	}

	if old != nil {
		old.waitInFlight(ctx)
		if err := old.Close(); err != nil {
			x.config.Logger.Warn("error closing instance providers on shutdown", slog.Any("error", err))
		}
	}

	return nil
}

// Stop is immediate teardown: no drain wait, then the same path as [Shutdown].
func (x *Server) Stop() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = x.Shutdown(ctx)
}

// retire cancels an instance, waits for in-flight requests (or grace), then Closes.
// Must not be called under x.mutex (wait can take up to grace).
func (x *Server) retire(old *Instance, oldCancel context.CancelFunc, graceCtx context.Context) {
	if old == nil {
		return
	}
	if oldCancel != nil {
		oldCancel()
	}
	old.waitInFlight(graceCtx)
	if err := old.Close(); err != nil {
		x.config.Logger.Warn("error closing previous instance providers", slog.Any("error", err))
	}
}
