package test

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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oakwood-commons/event-reactor/pkg/adapter"
	"github.com/oakwood-commons/event-reactor/pkg/api"
	"github.com/oakwood-commons/event-reactor/pkg/config"
	"github.com/oakwood-commons/event-reactor/pkg/matcher"
	"github.com/oakwood-commons/event-reactor/pkg/message"
	"github.com/oakwood-commons/event-reactor/pkg/reactor"
)

type echoProvider struct{}

func (e *echoProvider) Name() string { return "echo" }
func (e *echoProvider) Execute(_ context.Context, inputs map[string]any, _ message.Event) (*reactor.Result, error) {
	return &reactor.Result{Provider: "echo", Output: inputs}, nil
}

func TestSmoke_ServerEndToEnd(t *testing.T) {
	// Load the smoke config
	cfg, err := config.Load("testdata/smoke-config.yaml")
	require.NoError(t, err)

	// Set up components
	m, err := matcher.New()
	require.NoError(t, err)

	reg := reactor.NewRegistry()
	reg.Register(&echoProvider{})

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	a := adapter.New(cfg, m, reg, logger)
	srv := api.New(cfg, a, logger)

	// Health check
	t.Run("liveness", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/health/live", nil)
		srv.Router().ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("readiness", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/health/ready", nil)
		srv.Router().ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// Send event through full pipeline
	t.Run("event_pipeline", func(t *testing.T) {
		body := `{"action": "test", "source": "smoke"}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		srv.Router().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Processed int              `json:"processed"`
			Results   []map[string]any `json:"results"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.Processed)
		require.Len(t, resp.Results, 1)

		result := resp.Results[0]
		assert.Equal(t, "echo", result["provider"])
		assert.Equal(t, "smoke-echo", result["reactorName"])

		// Verify resolved inputs came through
		output, ok := result["output"].(map[string]any)
		require.True(t, ok, "output should be a map, got %T", result["output"])
		assert.Equal(t, "smoke test passed", output["message"])
	})

	// Multiple events
	t.Run("multiple_events", func(t *testing.T) {
		for i := range 5 {
			body := fmt.Sprintf(`{"iteration": %d}`, i)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			srv.Router().ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
		}
	})
}
