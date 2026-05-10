package generic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oakwood-commons/event-reactor/pkg/message"
)

func TestGenericListener(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	l := New(Config{Name: "test-generic", Port: 19876, Path: "/push"}, logger)
	assert.Equal(t, "test-generic", l.Name())

	received := make(chan message.Event, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = l.Start(ctx, func(_ context.Context, e message.Event) {
			received <- e
		})
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	payload := map[string]any{"action": "deployed", "service": "api"}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(fmt.Sprintf("http://localhost:19876/push"), "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	select {
	case e := <-received:
		assert.Equal(t, "deployed", e.Payload.(map[string]any)["action"])
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}
