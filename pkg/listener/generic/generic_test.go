package generic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oakwood-commons/event-reactor/pkg/message"
)

func testListener(t *testing.T) *Listener {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return New(Config{Name: "test-generic", Host: "127.0.0.1", Port: 0, Path: "/push"}, logger)
}

func TestGenericListener_Name(t *testing.T) {
	l := testListener(t)
	assert.Equal(t, "test-generic", l.Name())
}

func TestGenericListener_DefaultPath(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	l := New(Config{Name: "test-default", Port: 0, Path: ""}, logger)
	assert.Equal(t, "/events", l.path)
}

func TestStart_AcceptsAndShutdown(t *testing.T) {
	l := testListener(t)

	received := make(chan message.Event, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- l.Start(ctx, func(_ context.Context, e message.Event) {
			received <- e
		})
	}()

	// Addr() blocks until the listener is bound and serving
	addr := l.Addr()
	require.NotEmpty(t, addr, "listener should have a bound address")

	payload := map[string]any{"action": "deployed"}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, fmt.Sprintf("http://%s/push", addr), bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	select {
	case e := <-received:
		p, ok := e.Payload.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deployed", p["action"])
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	// Shutdown
	cancel()
	assert.NoError(t, <-errCh)
}

func TestHandlePush_Success(t *testing.T) {
	l := testListener(t)

	var received message.Event
	l.handler = func(_ context.Context, e message.Event) {
		received = e
	}

	payload := map[string]any{"action": "deployed", "service": "api"}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/push", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	l.handlePush(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	require.NotNil(t, received.Payload)
	p, ok := received.Payload.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "deployed", p["action"])
}

func TestHandlePush_MethodNotAllowed(t *testing.T) {
	l := testListener(t)
	l.handler = func(_ context.Context, _ message.Event) {}

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/push", nil)
	l.handlePush(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandlePush_InvalidJSON(t *testing.T) {
	l := testListener(t)
	l.handler = func(_ context.Context, _ message.Event) {}

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/push", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	l.handlePush(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
