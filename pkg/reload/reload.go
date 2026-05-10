// Package reload provides config file hot-reload via fsnotify.
package reload

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/oakwood-commons/event-reactor/pkg/config"
)

// Callback is called when config is successfully reloaded.
type Callback func(*config.ServerConfig)

// Watcher watches a config file and triggers reload on changes.
type Watcher struct {
	path     string
	callback Callback
	logger   *slog.Logger
	mu       sync.Mutex
	last     time.Time
	debounce time.Duration
}

// New creates a config file watcher.
func New(path string, cb Callback, logger *slog.Logger) *Watcher {
	return &Watcher{
		path:     path,
		callback: cb,
		logger:   logger,
		debounce: 500 * time.Millisecond,
	}
}

// Watch starts watching the config file. Blocks until context is cancelled.
func (w *Watcher) Watch(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	if err := watcher.Add(w.path); err != nil {
		return err
	}

	w.logger.Info("watching config for changes", slog.String("path", w.path))

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				w.handleChange()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			w.logger.Error("watcher error", slog.String("error", err.Error()))
		}
	}
}

func (w *Watcher) handleChange() {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	if now.Sub(w.last) < w.debounce {
		return
	}
	w.last = now

	cfg, err := config.Load(w.path)
	if err != nil {
		w.logger.Error("config reload failed", slog.String("error", err.Error()))
		return
	}

	w.logger.Info("config reloaded successfully")
	w.callback(cfg)
}
