package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/health/live", nil)
	s.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

func TestHealthReadiness(t *testing.T) {
	s := testServer(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/health/ready", nil)
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
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/events", bytes.NewBufferString(body))
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
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/events", bytes.NewBufferString("not json"))
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
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/events", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	s.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["processed"])
}

func TestHandleCloudEvent_InvalidJSON(t *testing.T) {
	srv := testServer(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/cloudevents", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid JSON payload")
}

func TestHandleCloudEvent_StructuredNoData(t *testing.T) {
	srv := testServer(t, config.ReactorConfig{
		Name:     "test-echo",
		Match:    "true",
		Provider: "echo",
		Inputs: map[string]config.InputValue{
			"msg": config.NewInputStatic("hello"),
		},
	})

	// CloudEvent with no "data" field -- entire body becomes payload
	ce := map[string]any{
		"specversion": "1.0",
		"id":          "no-data-001",
		"source":      "/test/source",
		"type":        "com.example.nodata",
	}
	body, _ := json.Marshal(ce)

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/cloudevents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["processed"])
}

func TestHandleCloudEvent_BinaryInvalidJSON(t *testing.T) {
	srv := testServer(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/cloudevents", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Ce-Type", "com.example.test")
	req.Header.Set("Ce-Source", "/test")
	req.Header.Set("Ce-Id", "bad-001")
	req.Header.Set("Ce-Specversion", "1.0")
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleWebhook_XSignature256Header(t *testing.T) {
	srv := testServer(t)
	srv.cfg.Auth.WebhookSecrets = []config.WebhookSecret{{Source: "alt", Secret: "altsecret"}}

	payload := []byte(`{"event":"test"}`)
	mac := hmac.New(sha256.New, []byte("altsecret"))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/webhook/alt", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature-256", sig) // alternative header
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleWebhook_MissingSignature(t *testing.T) {
	srv := testServer(t)
	srv.cfg.Auth.WebhookSecrets = []config.WebhookSecret{{Source: "secure", Secret: "mysecret"}}

	payload := []byte(`{"event":"deploy"}`)

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/webhook/secure", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	// No signature header at all
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandleWebhook_InvalidJSON(t *testing.T) {
	srv := testServer(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/webhook/github", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid JSON payload")
}

func TestHandleWebhook_GitHubEventHeader(t *testing.T) {
	srv := testServer(t, config.ReactorConfig{
		Name:     "test-echo",
		Match:    "true",
		Provider: "echo",
		Inputs: map[string]config.InputValue{
			"msg": config.NewInputStatic("hello"),
		},
	})

	payload := []byte(`{"action":"opened"}`)

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/webhook/github", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["processed"])
}
