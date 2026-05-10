package providers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oakwood-commons/event-reactor/pkg/message"
	"github.com/oakwood-commons/event-reactor/pkg/reactor"
)

func testEvent() message.Event {
	return message.Event{ID: "test-1", Source: "test", Payload: map[string]any{"action": "test"}}
}

func TestEcho(t *testing.T) {
	p := Echo{}
	assert.Equal(t, "echo", p.Name())

	result, err := p.Execute(context.Background(), map[string]any{"msg": "hello"}, testEvent())
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"msg": "hello"}, result.Output)
}

func TestHTTP_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer ts.Close()

	p := NewHTTP()
	result, err := p.Execute(context.Background(), map[string]any{
		"url":  ts.URL,
		"body": map[string]any{"key": "value"},
	}, testEvent())
	require.NoError(t, err)
	out, ok := result.Output.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 200, out["statusCode"])
}

func TestHTTP_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	p := NewHTTP()
	_, err := p.Execute(context.Background(), map[string]any{"url": ts.URL}, testEvent())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestHTTP_MissingURL(t *testing.T) {
	p := NewHTTP()
	_, err := p.Execute(context.Background(), map[string]any{}, testEvent())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "url is required")
}

func TestHTTP_AuthHeaderInjection(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	p := NewHTTP()
	ctx := reactor.WithAuthHeader(context.Background(), "Bearer secret-token")
	result, err := p.Execute(ctx, map[string]any{
		"url": ts.URL,
	}, testEvent())
	require.NoError(t, err)
	out, ok := result.Output.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 200, out["statusCode"])
}

func TestHTTP_StringBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "text/plain", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	p := NewHTTP()
	_, err := p.Execute(context.Background(), map[string]any{
		"url":         ts.URL,
		"body":        "plain text",
		"contentType": "text/plain",
	}, testEvent())
	require.NoError(t, err)
}

func TestExec_Success(t *testing.T) {
	p := &Exec{Logger: slog.New(slog.NewTextHandler(os.Stderr, nil))}
	assert.Equal(t, "exec", p.Name())

	var cmd string
	var args []any
	if os.PathSeparator == '\\' {
		cmd = "cmd"
		args = []any{"/c", "echo hello"}
	} else {
		cmd = "echo"
		args = []any{"hello"}
	}

	result, err := p.Execute(context.Background(), map[string]any{
		"command": cmd,
		"args":    args,
	}, testEvent())
	require.NoError(t, err)
	out, ok := result.Output.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, out["stdout"], "hello")
	assert.Equal(t, 0, out["exitCode"])
}

func TestLog(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	p := &Log{Logger: logger}
	assert.Equal(t, "log", p.Name())

	result, err := p.Execute(context.Background(), map[string]any{"level": "info", "message": "test"}, testEvent())
	require.NoError(t, err)
	out, ok := result.Output.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, out["logged"])
}

func TestRegisterAll(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	reg := reactor.NewRegistry()
	RegisterAll(reg, logger)

	providers := reg.Providers()
	assert.Contains(t, providers, "echo")
	assert.Contains(t, providers, "http")
	assert.Contains(t, providers, "exec")
	assert.Contains(t, providers, "log")
}
