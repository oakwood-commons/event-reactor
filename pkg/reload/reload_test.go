package reload

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oakwood-commons/event-reactor/pkg/config"
)

func TestWatcher_ReloadsOnWrite(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	initial := `
reactors:
  - name: first
    match: "true"
    provider: echo
    inputs:
      msg: hello
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(initial), 0o600))

	var reloaded *config.ServerConfig
	done := make(chan struct{})

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	w := New(cfgPath, func(cfg *config.ServerConfig) {
		reloaded = cfg
		close(done)
	}, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = w.Watch(ctx)
	}()

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Write updated config
	updated := `
reactors:
  - name: second
    match: "true"
    provider: http
    inputs:
      url: https://example.com
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(updated), 0o600))

	select {
	case <-done:
		require.NotNil(t, reloaded)
		assert.Len(t, reloaded.Reactors, 1)
		assert.Equal(t, "second", reloaded.Reactors[0].Name)
	case <-ctx.Done():
		t.Fatal("timeout waiting for reload")
	}
}
