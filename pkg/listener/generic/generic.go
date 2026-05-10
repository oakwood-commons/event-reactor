// Package generic implements a generic HTTP push listener that accepts
// events via an internal HTTP endpoint. Used for testing and custom integrations.
package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/oakwood-commons/event-reactor/pkg/message"
)

const readHeaderTimeout = 5 * time.Second

// Listener accepts events via HTTP POST on the configured port/path.
type Listener struct {
	name    string
	port    int
	path    string
	logger  *slog.Logger
	handler func(context.Context, message.Event)
}

// Config for the generic HTTP listener.
type Config struct {
	Name string
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
		port:   cfg.Port,
		path:   cfg.Path,
		logger: logger,
	}
}

func (l *Listener) Name() string { return l.name }

func (l *Listener) Start(ctx context.Context, handler func(context.Context, message.Event)) error {
	l.handler = handler

	mux := http.NewServeMux()
	mux.HandleFunc(l.path, l.handlePush)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", l.port),
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		l.logger.Info("generic listener started", slog.Int("port", l.port), slog.String("path", l.path))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
