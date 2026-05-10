// Package observability sets up structured logging, Prometheus metrics,
// and OpenTelemetry tracing for event-reactor.
package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/oakwood-commons/event-reactor/pkg/config"
)

// Logger creates a structured slog.Logger from the observability config.
func Logger(cfg config.LoggingConfig) *slog.Logger {
	level := parseLevel(cfg.Level)
	var handler slog.Handler

	w := writerForFormat(cfg.Format)
	switch cfg.Format {
	case "text":
		handler = slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})
	default:
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	}

	return slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func writerForFormat(_ string) io.Writer {
	return os.Stdout
}

// Shutdown is a collection of cleanup functions for observability resources.
type Shutdown struct {
	fns []func(context.Context) error
}

// Add registers a shutdown function.
func (s *Shutdown) Add(fn func(context.Context) error) {
	s.fns = append(s.fns, fn)
}

// Run executes all shutdown functions, collecting errors.
func (s *Shutdown) Run(ctx context.Context) error {
	var errs []error
	for _, fn := range s.fns {
		if err := fn(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}
