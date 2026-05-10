package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oakwood-commons/event-reactor/pkg/adapter"
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

func testServer(t *testing.T, reactors ...config.ReactorConfig) *Server {
	t.Helper()
	m, err := matcher.New()
	require.NoError(t, err)

	reg := reactor.NewRegistry()
	reg.Register(&echoProvider{})

	cfg := &config.ServerConfig{
		Server: config.ServerSettings{
			Port:        8080,
			MetricsPort: 9090,
			HealthCheck: config.HealthCheckConfig{
				Liveness:  "/health/live",
				Readiness: "/health/ready",
			},
		},
		Reactors: reactors,
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	a := adapter.New(cfg, m, reg, logger)
	return New(cfg, a, logger)
}

func TestHealthLiveness(t *testing.T) {
	s := testServer(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health/live", nil)
	s.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

func TestHealthReadiness(t *testing.T) {
	s := testServer(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health/ready", nil)
	s.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ready")
}

func TestHandleEvent_ValidPayload(t *testing.T) {
	s := testServer(t, config.ReactorConfig{
		Name:     "test-echo",
		Match:    "true",
		Provider: "echo",
		Inputs: map[string]config.InputValue{
			"msg": config.NewInputStatic("hello"),
		},
	})

	body := `{"action": "opened"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	s.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["processed"])
}

func TestHandleEvent_InvalidJSON(t *testing.T) {
	s := testServer(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/events", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	s.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid JSON payload")
}

func TestHandleEvent_NoMatchingReactors(t *testing.T) {
	s := testServer(t, config.ReactorConfig{
		Name:     "no-match",
		Match:    `payload.action == "closed"`,
		Provider: "echo",
		Inputs:   map[string]config.InputValue{},
	})

	body := `{"action": "opened"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	s.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["processed"])
}
