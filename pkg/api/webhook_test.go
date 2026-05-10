package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oakwood-commons/event-reactor/pkg/config"
)

func TestHandleCloudEvent_Structured(t *testing.T) {
	srv := testServer(t)
	router := srv.Router()

	ce := map[string]any{
		"specversion": "1.0",
		"id":          "ce-123",
		"source":      "/test/source",
		"type":        "com.example.test",
		"data": map[string]any{
			"action": "test",
		},
	}
	body, _ := json.Marshal(ce)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/cloudevents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "processed")
}

func TestHandleCloudEvent_Binary(t *testing.T) {
	srv := testServer(t)
	router := srv.Router()

	body, _ := json.Marshal(map[string]any{"action": "opened"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/cloudevents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Ce-Id", "bin-456")
	req.Header.Set("Ce-Source", "/binary/source")
	req.Header.Set("Ce-Type", "com.example.binary")
	req.Header.Set("Ce-Specversion", "1.0")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleWebhook_NoSecret(t *testing.T) {
	srv := testServer(t)
	router := srv.Router()

	body, _ := json.Marshal(map[string]any{"event": "push"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/webhook/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "push")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "processed")
}

func TestHandleWebhook_ValidHMAC(t *testing.T) {
	srv := testServer(t)
	srv.cfg.Auth.WebhookSecrets = []config.WebhookSecret{{Source: "secure", Secret: "mysecret"}}
	router := srv.Router()

	payload := []byte(`{"event":"deploy"}`)
	mac := hmac.New(sha256.New, []byte("mysecret"))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/webhook/secure", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", sig)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleWebhook_InvalidHMAC(t *testing.T) {
	srv := testServer(t)
	srv.cfg.Auth.WebhookSecrets = []config.WebhookSecret{{Source: "secure", Secret: "mysecret"}}
	router := srv.Router()

	payload := []byte(`{"event":"deploy"}`)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/webhook/secure", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
