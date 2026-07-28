package xtemplate

import (
	"context"
	"errors"
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

// Server is a reloadable http.Handler that always routes to the current
// [Instance], or responds 503 when none is loaded or the server has stopped.
//
// Optional [ServerController]: Init supplies sticky base options, then Start
// may drive reloads. The sticky base is fixed at construction.
//
// [Server.Reload] rebuilds from sticky plus ephemeral options. [Server.Shutdown]
// / [Server.Stop] tear down; with [Server.Serve], cancelling the server context
// also drains the local http.Server (not stored on Server).
//
// Create only via [Config.Server].
type Server struct {
	instance atomic.Pointer[Instance]
	cancel   context.CancelFunc // cancels current instance ctx

	mutex  sync.Mutex
	config Config

	serverCtx    context.Context
	serverCancel context.CancelFunc
}

var _ http.Handler = (*Server)(nil)

// Context is cancelled on Stop/Shutdown. Controllers use it to halt background work.
func (server *Server) Context() context.Context {
	return server.serverCtx
}

// Logger returns the server logger (xtemplate group applied at construction).
func (server *Server) Logger() *slog.Logger {
	return server.config.Logger
}

// Server creates a Server from Config. Apply Options on the Config first; this
// method does not take Option args. Owned slices are cloned so the caller's
// Config is not mutated by sticky Options or later Reloads.
func (config Config) Server() (*Server, error) {
	config.SetDefaults()
	config.cloneSlices()
	config.Logger = config.Logger.WithGroup("xtemplate")

	serverCtx, serverCancel := context.WithCancel(config.Ctx)
	server := &Server{
		config:       config,
		serverCtx:    serverCtx,
		serverCancel: serverCancel,
	}

	if err := server.construct(); err != nil {
		serverCancel()
		return nil, err
	}

	if server.config.Controller != nil {
		if err := server.config.Controller.Start(server); err != nil {
			server.Stop()
			return nil, err
		}
	}

	return server, nil
}

// construct resolves the controller, applies Init sticky options, and loads the
// first Instance when TemplateFS is set. No mutex: Server is not published yet.
// Caller owns serverCancel on error.
func (server *Server) construct() error {
	// Materialize when already set, from Raw, or default when no TemplateFS.
	// Leave Controller nil when TemplateFS is set and no controller was configured
	// (in-process FS / deferred controllers that only Reload).
	if server.config.Controller != nil || len(server.config.ControllerRaw) != 0 || server.config.TemplateFS == nil {
		if _, err := server.config.MaterializeController(""); err != nil {
			return err
		}
	}

	if server.config.Controller != nil {
		log := server.config.Logger.WithGroup("controller")
		sticky, err := server.config.Controller.Init(server.serverCtx, log)
		if err != nil {
			return err
		}
		if _, err = server.config.Options(sticky...); err != nil {
			return err
		}
	}

	if server.config.TemplateFS != nil {
		return server.Reload()
	}
	server.config.Logger.Info("TemplateFS not set, server will respond with 503 until the first successful reload")
	return nil
}

// Reload builds a new Instance from the sticky base plus options and swaps it
// in on success. Previous instance is cancelled, drained up to [defaultGrace],
// then closed (outside the mutex so concurrent Reload/Shutdown are not blocked).
// Ephemeral WithTemplateFS/Dir apply only to this build; empty Reload rebuilds
// from sticky. Fails if the final template root is nil. WithController is rejected.
func (server *Server) Reload(options ...Option) error {
	start := time.Now()

	server.mutex.Lock()

	if server.serverCtx.Err() != nil {
		server.mutex.Unlock()
		config, optErr := New().Options(options...)
		return errors.Join(errors.New("server stopped"), optErr, onCloseFunc(&config.onClose)())
	}

	log := server.config.Logger.WithGroup("reload")
	if prev := server.instance.Load(); prev != nil {
		log = log.With(slog.Int64("old_id", prev.id))
	}

	config := server.config
	config.cloneSlices()
	config.Controller = nil
	config.ControllerRaw = nil
	if _, err := config.Options(options...); err != nil {
		server.mutex.Unlock()
		return err
	}
	if config.TemplateFS == nil {
		server.mutex.Unlock()
		return errors.Join(
			errors.New("xtemplate: no template root (sticky unset and Reload options did not set WithTemplateFS/WithTemplateDir)"),
			onCloseFunc(&config.onClose)(),
		)
	}
	var newcancel context.CancelFunc
	config.Ctx, newcancel = context.WithCancel(server.serverCtx)
	new_, err := config.buildInstance()
	if err != nil {
		newcancel()
		server.mutex.Unlock()
		log.Info("failed to load", slog.Any("error", err), slog.Duration("rebuild_time", time.Since(start)))
		return err
	}

	old := server.instance.Swap(new_)
	oldCancel := server.cancel
	server.cancel = newcancel
	server.mutex.Unlock()

	log.Info("rebuild succeeded", slog.Int64("new_id", new_.id), slog.Duration("rebuild_time", time.Since(start)))

	if old != nil {
		graceCtx, graceCancel := context.WithTimeout(context.Background(), defaultGrace)
		server.retire(old, oldCancel, graceCtx)
		graceCancel()
	}
	return nil
}

// Instance returns the current [Instance]. After calling Reload, previous calls
// to Instance may be stale. When not ready yet or after Stop/Shutdown, returns nil.
func (server *Server) Instance() *Instance {
	return server.instance.Load()
}

// Serve opens a net listener on `listen_addr` and serves requests from it.
// It returns when the listener fails or when the server context is cancelled
// (parent [Config.Ctx] or [Server.Shutdown]/[Server.Stop]), in which case the
// local [http.Server] is drained (default grace [defaultGrace]), the instance
// is retired, and Serve returns nil.
func (server *Server) Serve(listen_addr string) error {
	ln, err := net.Listen("tcp", listen_addr)
	if err != nil {
		return err
	}
	// Log the actual bound address (resolved from listen_addr) so the port is
	// visible in the logs, including when listen_addr requests an ephemeral
	// port like ":0".
	server.config.Logger.Info("starting server", slog.String("address", ln.Addr().String()))

	srv := &http.Server{
		Handler:           server,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		<-server.serverCtx.Done()
		drainCtx, cancel := context.WithTimeout(context.Background(), defaultGrace)
		defer cancel()
		// Retire instance first (serverCtx already cancelled → SSE unblocks),
		// then drain this Serve-local http.Server. Server does not own *http.Server.
		_ = server.Shutdown(drainCtx)
		_ = srv.Shutdown(drainCtx)
	}()

	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// ServeHTTP routes the request to the current [Instance], or responds 503 if
// no instance is loaded yet or the server has been stopped.
func (server *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	instance := server.Instance()
	if instance == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
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
func (server *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	server.mutex.Lock()
	old := server.instance.Swap(nil)
	oldCancel := server.cancel
	server.cancel = nil

	if server.serverCancel != nil {
		server.serverCancel()
	}
	server.mutex.Unlock()

	// Cancel instance explicitly as well (no-op if already cancelled via serverCtx).
	if oldCancel != nil {
		oldCancel()
	}

	if old != nil {
		old.waitInFlight(ctx)
		if err := old.Close(); err != nil {
			server.config.Logger.Warn("error closing instance providers on shutdown", slog.Any("error", err))
		}
	}

	return nil
}

// Stop is immediate teardown: no drain wait, then the same path as [Shutdown].
func (server *Server) Stop() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = server.Shutdown(ctx)
}

// retire cancels an instance, waits for in-flight requests (or grace), then Closes.
// Caller must not hold server.mutex.
func (server *Server) retire(old *Instance, oldCancel context.CancelFunc, graceCtx context.Context) {
	if old == nil {
		return
	}
	if oldCancel != nil {
		oldCancel()
	}
	old.waitInFlight(graceCtx)
	if err := old.Close(); err != nil {
		server.config.Logger.Warn("error closing previous instance providers", slog.Any("error", err))
	}
}
