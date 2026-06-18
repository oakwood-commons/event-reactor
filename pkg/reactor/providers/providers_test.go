package providers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"os"
	"testing"
	"time"

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
	assert.Contains(t, providers, "smtp")
}

// --- SMTP tests ---

type mockSMTPClient struct {
	startTLSCalled bool
	authCalled     bool
	mailFrom       string
	rcptTo         []string
	dataWritten    []byte
	quitCalled     bool

	startTLSErr error
	authErr     error
	mailErr     error
	rcptErr     error
	dataErr     error
	writeErr    error
	closeErr    error
	quitErr     error
}

func (m *mockSMTPClient) StartTLS(_ *tls.Config) error {
	m.startTLSCalled = true
	return m.startTLSErr
}

func (m *mockSMTPClient) Auth(_ smtp.Auth) error {
	m.authCalled = true
	return m.authErr
}

func (m *mockSMTPClient) Mail(from string) error {
	m.mailFrom = from
	return m.mailErr
}

func (m *mockSMTPClient) Rcpt(to string) error {
	m.rcptTo = append(m.rcptTo, to)
	return m.rcptErr
}

func (m *mockSMTPClient) Data() (io.WriteCloser, error) {
	if m.dataErr != nil {
		return nil, m.dataErr
	}
	return &mockSMTPWriter{client: m}, nil
}

func (m *mockSMTPClient) Quit() error {
	m.quitCalled = true
	return m.quitErr
}

func (m *mockSMTPClient) Close() error { return nil }

type mockSMTPWriter struct {
	client *mockSMTPClient
}

func (w *mockSMTPWriter) Write(p []byte) (int, error) {
	if w.client.writeErr != nil {
		return 0, w.client.writeErr
	}
	w.client.dataWritten = append(w.client.dataWritten, p...)
	return len(p), nil
}

func (w *mockSMTPWriter) Close() error { return w.client.closeErr }

type mockSMTPDialer struct {
	client    *mockSMTPClient
	dialErr   error
	failCount int
	attempt   int
}

func (d *mockSMTPDialer) Dial(_ string) (SMTPClient, error) {
	d.attempt++
	if d.failCount > 0 && d.attempt <= d.failCount {
		return nil, d.dialErr
	}
	if d.dialErr != nil && d.failCount == 0 {
		return nil, d.dialErr
	}
	return d.client, nil
}

func TestSMTP_Name(t *testing.T) {
	p := &SMTP{Logger: slog.New(slog.NewTextHandler(os.Stderr, nil))}
	assert.Equal(t, "smtp", p.Name())
}

func TestSMTP_Success(t *testing.T) {
	client := &mockSMTPClient{}
	p := &SMTP{
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dialer: &mockSMTPDialer{client: client},
	}

	result, err := p.Execute(context.Background(), map[string]any{
		"host":     "smtp.example.com",
		"from":     "sender@example.com",
		"to":       []any{"r@example.com"},
		"subject":  "Test",
		"body":     "Hello",
		"username": "user",
		"password": "pass",
	}, testEvent())
	require.NoError(t, err)
	assert.Equal(t, "smtp", result.Provider)
	out, ok := result.Output.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, out["attempts"])
	assert.True(t, client.startTLSCalled)
	assert.True(t, client.authCalled)
	assert.Equal(t, "sender@example.com", client.mailFrom)
	assert.Contains(t, string(client.dataWritten), "Subject: Test\r\n")
}

func TestSMTP_MissingHost(t *testing.T) {
	p := &SMTP{Logger: slog.New(slog.NewTextHandler(os.Stderr, nil))}
	_, err := p.Execute(context.Background(), map[string]any{
		"from":    "s@example.com",
		"to":      "r@example.com",
		"subject": "T",
		"body":    "B",
	}, testEvent())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "host is required")
}

func TestSMTP_HeaderInjection(t *testing.T) {
	p := &SMTP{Logger: slog.New(slog.NewTextHandler(os.Stderr, nil))}

	_, err := p.Execute(context.Background(), map[string]any{
		"host":    "smtp.example.com",
		"from":    "s@example.com",
		"to":      []any{"r@example.com"},
		"subject": "Test\r\nBcc: evil@attacker.com",
		"body":    "body",
	}, testEvent())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subject contains invalid characters")
}

func TestSMTP_AuthFromContext(t *testing.T) {
	client := &mockSMTPClient{}
	p := &SMTP{
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dialer: &mockSMTPDialer{client: client},
	}

	ctx := reactor.WithAuthHeader(context.Background(), "smtp-secret-password")
	_, err := p.Execute(ctx, map[string]any{
		"host":     "smtp.example.com",
		"from":     "s@example.com",
		"to":       []any{"r@example.com"},
		"subject":  "Test",
		"body":     "Hello",
		"username": "user",
	}, testEvent())
	require.NoError(t, err)
	assert.True(t, client.authCalled)
}

func TestSMTP_AuthFromContext_BearerScheme(t *testing.T) {
	client := &mockSMTPClient{}
	p := &SMTP{
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dialer: &mockSMTPDialer{client: client},
	}

	// Auth header with Bearer scheme should extract just the token portion.
	ctx := reactor.WithAuthHeader(context.Background(), "Bearer my-token-value")
	_, err := p.Execute(ctx, map[string]any{
		"host":     "smtp.example.com",
		"from":     "s@example.com",
		"to":       []any{"r@example.com"},
		"subject":  "Test",
		"body":     "Hello",
		"username": "user",
	}, testEvent())
	require.NoError(t, err)
	assert.True(t, client.authCalled)
}

func TestSMTP_NegativeMaxRetries(t *testing.T) {
	client := &mockSMTPClient{}
	p := &SMTP{
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dialer: &mockSMTPDialer{client: client},
	}

	// Negative maxRetries should be clamped to 0, meaning one attempt (no retries).
	result, err := p.Execute(context.Background(), map[string]any{
		"host":       "smtp.example.com",
		"from":       "s@example.com",
		"to":         []any{"r@example.com"},
		"subject":    "Test",
		"body":       "Hello",
		"maxRetries": -5,
	}, testEvent())
	require.NoError(t, err)
	out, ok := result.Output.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, out["attempts"])
}

func TestSMTP_EmptyStringRecipients(t *testing.T) {
	p := &SMTP{
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dialer: &mockSMTPDialer{client: &mockSMTPClient{}},
	}

	// []string with only empty strings should be treated as missing recipients.
	_, err := p.Execute(context.Background(), map[string]any{
		"host":    "smtp.example.com",
		"from":    "s@example.com",
		"to":      []string{"", ""},
		"subject": "Test",
		"body":    "Hello",
	}, testEvent())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "to is required")
}

func TestSMTP_StartTLSCamelCase(t *testing.T) {
	client := &mockSMTPClient{}
	p := &SMTP{
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dialer: &mockSMTPDialer{client: client},
	}

	// "startTLS" (camelCase) should be accepted the same as "starttls".
	_, err := p.Execute(context.Background(), map[string]any{
		"host":     "smtp.example.com",
		"from":     "s@example.com",
		"to":       []any{"r@example.com"},
		"subject":  "Test",
		"body":     "Hello",
		"startTLS": true,
	}, testEvent())
	require.NoError(t, err)
	assert.True(t, client.startTLSCalled)
}

func TestSMTP_DialError(t *testing.T) {
	p := &SMTP{
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dialer: &mockSMTPDialer{dialErr: errors.New("connection refused")},
	}

	_, err := p.Execute(context.Background(), map[string]any{
		"host":    "smtp.example.com",
		"from":    "s@example.com",
		"to":      []any{"r@example.com"},
		"subject": "Test",
		"body":    "Hello",
	}, testEvent())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dialing SMTP server")
}

func TestSMTP_RetrySuccess(t *testing.T) {
	client := &mockSMTPClient{}
	p := &SMTP{
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dialer: &mockSMTPDialer{
			client:    client,
			dialErr:   errors.New("dialing SMTP server: connection reset"),
			failCount: 2,
		},
		NewTimer: func(_ time.Duration) *time.Timer { return time.NewTimer(0) }, // instant for tests
	}

	result, err := p.Execute(context.Background(), map[string]any{
		"host":       "smtp.example.com",
		"from":       "s@example.com",
		"to":         []any{"r@example.com"},
		"subject":    "Test",
		"body":       "Hello",
		"maxRetries": 3,
	}, testEvent())
	require.NoError(t, err)
	out, ok := result.Output.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 3, out["attempts"])
}

func TestSMTP_AuthError_NoRetry(t *testing.T) {
	client := &mockSMTPClient{authErr: errors.New("authenticating: bad credentials")}
	p := &SMTP{
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dialer: &mockSMTPDialer{client: client},
	}

	result, err := p.Execute(context.Background(), map[string]any{
		"host":       "smtp.example.com",
		"from":       "s@example.com",
		"to":         []any{"r@example.com"},
		"subject":    "Test",
		"body":       "Hello",
		"username":   "user",
		"password":   "wrong",
		"maxRetries": 3,
	}, testEvent())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authenticating")
	out, ok := result.Output.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, out["attempts"]) // No retry on auth failure
}
