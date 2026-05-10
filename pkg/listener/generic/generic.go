// Package generic implements a generic HTTP push listener that accepts
// events via an internal HTTP endpoint. Used for testing and custom integrations.
package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/oakwood-commons/event-reactor/pkg/message"
)

const readHeaderTimeout = 5 * time.Second

// Listener accepts events via HTTP POST on the configured port/path.
type Listener struct {
	name    string
	host    string
	port    int
	path    string
	logger  *slog.Logger
	handler func(context.Context, message.Event)
	addr    string        // actual address after Start binds
	ready   chan struct{} // closed when the listener is bound and serving
}

// Config for the generic HTTP listener.
type Config struct {
	Name string
	Host string // bind address (default: all interfaces)
	Port int
	Path string
}

// New creates a generic HTTP push listener.
func New(cfg Config, logger *slog.Logger) *Listener {
	if cfg.Path == "" {
		cfg.Path = "/events"
	}
	return &Listener{
		name:   cfg.Name,
		host:   cfg.Host,
		port:   cfg.Port,
		path:   cfg.Path,
		logger: logger,
		ready:  make(chan struct{}),
	}
}

func (l *Listener) Name() string { return l.name }

// Addr returns the listener's bound address. Blocks until the listener is ready.
func (l *Listener) Addr() string {
	<-l.ready
	return l.addr
}

func (l *Listener) Start(ctx context.Context, handler func(context.Context, message.Event)) error {
	l.handler = handler

	mux := http.NewServeMux()
	mux.HandleFunc(l.path, l.handlePush)

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", net.JoinHostPort(l.host, strconv.Itoa(l.port)))
	if err != nil {
		close(l.ready)
		return fmt.Errorf("listening on %s:%d: %w", l.host, l.port, err)
	}
	l.addr = ln.Addr().String()

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		l.logger.Info("generic listener started", slog.String("addr", l.addr), slog.String("path", l.path))
		close(l.ready)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (l *Listener) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	event, err := message.FromGenericPayload(payload)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid event: %v", err), http.StatusBadRequest)
		return
	}

	l.handler(r.Context(), event)
	w.WriteHeader(http.StatusAccepted)
}
