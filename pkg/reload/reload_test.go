package reload

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

func TestWatcher_Debounce(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	initial := `
reactors:
  - name: test
    match: "true"
    provider: echo
    inputs:
      msg: hello
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(initial), 0o600))

	var callCount int
	var mu sync.Mutex

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	w := New(cfgPath, func(_ *config.ServerConfig) {
		mu.Lock()
		callCount++
		mu.Unlock()
	}, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = w.Watch(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Write rapidly -- debounce should collapse these
	for i := range 5 {
		updated := fmt.Sprintf(`
reactors:
  - name: test-%d
    match: "true"
    provider: echo
    inputs:
      msg: hello
`, i)
		require.NoError(t, os.WriteFile(cfgPath, []byte(updated), 0o600))
	}

	// Wait enough for debounce to settle
	time.Sleep(700 * time.Millisecond)
	cancel()

	mu.Lock()
	defer mu.Unlock()
	// Debounce should have collapsed multiple writes; expect fewer callbacks than writes
	assert.LessOrEqual(t, callCount, 3, "debounce should reduce callback count")
	assert.GreaterOrEqual(t, callCount, 1, "at least one reload should happen")
}

func TestWatcher_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	initial := `
reactors:
  - name: test
    match: "true"
    provider: echo
    inputs:
      msg: hello
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(initial), 0o600))

	var callCount atomic.Int32
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	w := New(cfgPath, func(_ *config.ServerConfig) {
		callCount.Add(1)
	}, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = w.Watch(ctx)
	}()

	// Let watcher settle and ignore any spurious events from initial setup
	time.Sleep(200 * time.Millisecond)
	callCount.Store(0)

	// Write invalid config
	require.NoError(t, os.WriteFile(cfgPath, []byte("invalid: [yaml: {"), 0o600))

	time.Sleep(700 * time.Millisecond)
	cancel()

	// Callback should NOT have been called for invalid config
	assert.Equal(t, int32(0), callCount.Load(), "callback should not fire for invalid config")
}

func TestHandleChange_Debounce_Direct(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	cfgYAML := `
reactors:
  - name: test
    match: "true"
    provider: echo
    inputs:
      msg: hello
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgYAML), 0o600))

	var callCount int
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	w := New(cfgPath, func(_ *config.ServerConfig) {
		callCount++
	}, logger)

	// First call should succeed
	w.handleChange()
	assert.Equal(t, 1, callCount)

	// Immediate second call should be debounced
	w.handleChange()
	assert.Equal(t, 1, callCount, "second call should be debounced")
}
