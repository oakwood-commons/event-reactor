package adapter

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oakwood-commons/event-reactor/pkg/auth"
	"github.com/oakwood-commons/event-reactor/pkg/config"
	"github.com/oakwood-commons/event-reactor/pkg/matcher"
	"github.com/oakwood-commons/event-reactor/pkg/message"
	"github.com/oakwood-commons/event-reactor/pkg/reactor"
)

type echoProvider struct{}

func (e *echoProvider) Name() string { return "echo" }
func (e *echoProvider) Execute(_ context.Context, inputs map[string]any, _ message.Event) (*reactor.Result, error) {
	return &reactor.Result{
		Provider: "echo",
		Output:   inputs,
	}, nil
}

// contextCaptureProvider records the auth header from context during execution.
type contextCaptureProvider struct {
	lastAuthHeader string
}

func (c *contextCaptureProvider) Name() string { return "capture" }
func (c *contextCaptureProvider) Execute(ctx context.Context, inputs map[string]any, _ message.Event) (*reactor.Result, error) {
	if h, ok := reactor.AuthHeader(ctx); ok {
		c.lastAuthHeader = h
	}
	return &reactor.Result{
		Provider: "capture",
		Output:   inputs,
	}, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestAdapter_HandleEvent_MatchAndDispatch(t *testing.T) {
	m, err := matcher.New()
	require.NoError(t, err)

	reg := reactor.NewRegistry()
	reg.Register(&echoProvider{})

	cfg := &config.ServerConfig{
		Reactors: []config.ReactorConfig{
			{
				Name:     "matching",
				Match:    `payload.action == "opened"`,
				Provider: "echo",
				Inputs: map[string]config.InputValue{
					"key": config.NewInputStatic("value"),
				},
			},
			{
				Name:     "not-matching",
				Match:    `payload.action == "closed"`,
				Provider: "echo",
				Inputs: map[string]config.InputValue{
					"key": config.NewInputStatic("other"),
				},
			},
		},
	}

	a := New(cfg, m, reg, testLogger())
	ev := message.Event{
		Payload: map[string]any{"action": "opened"},
	}

	results := a.HandleEvent(context.Background(), ev)
	require.Len(t, results, 1)
	assert.Equal(t, "matching", results[0].ReactorName)
	assert.Equal(t, "echo", results[0].Provider)
	assert.Empty(t, results[0].Error)
}

func TestAdapter_HandleEvent_DisabledReactor(t *testing.T) {
	m, err := matcher.New()
	require.NoError(t, err)

	reg := reactor.NewRegistry()
	reg.Register(&echoProvider{})

	cfg := &config.ServerConfig{
		Reactors: []config.ReactorConfig{
			{
				Name:     "disabled",
				Match:    "true",
				Provider: "echo",
				Disabled: true,
				Inputs:   map[string]config.InputValue{},
			},
		},
	}

	a := New(cfg, m, reg, testLogger())
	results := a.HandleEvent(context.Background(), message.Event{})
	assert.Empty(t, results)
}

func TestAdapter_HandleEvent_ProviderNotFound(t *testing.T) {
	m, err := matcher.New()
	require.NoError(t, err)

	reg := reactor.NewRegistry() // empty registry

	cfg := &config.ServerConfig{
		Reactors: []config.ReactorConfig{
			{
				Name:     "missing-provider",
				Match:    "true",
				Provider: "nonexistent",
				Inputs:   map[string]config.InputValue{},
			},
		},
	}

	a := New(cfg, m, reg, testLogger())
	results := a.HandleEvent(context.Background(), message.Event{})
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "provider lookup")
}

func TestAdapter_HandleEvent_MultipleMatches(t *testing.T) {
	m, err := matcher.New()
	require.NoError(t, err)

	reg := reactor.NewRegistry()
	reg.Register(&echoProvider{})

	cfg := &config.ServerConfig{
		Reactors: []config.ReactorConfig{
			{
				Name:     "reactor-a",
				Match:    "true",
				Provider: "echo",
				Inputs:   map[string]config.InputValue{"id": config.NewInputStatic("a")},
			},
			{
				Name:     "reactor-b",
				Match:    "true",
				Provider: "echo",
				Inputs:   map[string]config.InputValue{"id": config.NewInputStatic("b")},
			},
		},
	}

	a := New(cfg, m, reg, testLogger())
	results := a.HandleEvent(context.Background(), message.Event{})
	assert.Len(t, results, 2)

	names := make(map[string]bool)
	for _, r := range results {
		names[r.ReactorName] = true
	}
	assert.True(t, names["reactor-a"])
	assert.True(t, names["reactor-b"])
}

func TestAdapter_HandleEvent_NoReactors(t *testing.T) {
	m, err := matcher.New()
	require.NoError(t, err)

	cfg := &config.ServerConfig{}
	a := New(cfg, m, reactor.NewRegistry(), testLogger())
	results := a.HandleEvent(context.Background(), message.Event{})
	assert.Empty(t, results)
}

func TestAdapter_WithAuth_InjectsToken(t *testing.T) {
	m, err := matcher.New()
	require.NoError(t, err)

	// Use a provider that captures the context to verify auth header
	captureProvider := &contextCaptureProvider{}
	reg := reactor.NewRegistry()
	reg.Register(captureProvider)

	cfg := &config.ServerConfig{
		Reactors: []config.ReactorConfig{
			{
				Name:     "authed",
				Match:    "true",
				Provider: "capture",
				Auth:     "my-token",
				Inputs:   map[string]config.InputValue{},
			},
		},
	}

	authReg, err := auth.NewRegistry([]config.AuthHandlerConfig{
		{
			Name:   "my-token",
			Type:   "static-token",
			Config: map[string]any{"token": "secret123"},
		},
	})
	require.NoError(t, err)

	a := New(cfg, m, reg, testLogger()).WithAuth(authReg)

	results := a.HandleEvent(context.Background(), message.Event{})
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Error)

	// Auth header should be in context, not in inputs
	assert.Equal(t, "Bearer secret123", captureProvider.lastAuthHeader)
	output, ok := results[0].Output.(map[string]any)
	require.True(t, ok)
	_, hasAuthInInputs := output["_authHeader"]
	assert.False(t, hasAuthInInputs, "auth header should not leak into provider inputs")
}

func TestAdapter_WithAuth_HandlerNotFound(t *testing.T) {
	m, err := matcher.New()
	require.NoError(t, err)

	reg := reactor.NewRegistry()
	reg.Register(&echoProvider{})

	cfg := &config.ServerConfig{
		Reactors: []config.ReactorConfig{
			{
				Name:     "bad-auth",
				Match:    "true",
				Provider: "echo",
				Auth:     "nonexistent",
				Inputs:   map[string]config.InputValue{},
			},
		},
	}

	authReg, err := auth.NewRegistry(nil)
	require.NoError(t, err)

	a := New(cfg, m, reg, testLogger()).WithAuth(authReg)

	results := a.HandleEvent(context.Background(), message.Event{})
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "auth token")
}

func TestAdapter_WithAuth_NoRegistry(t *testing.T) {
	m, err := matcher.New()
	require.NoError(t, err)

	reg := reactor.NewRegistry()
	reg.Register(&echoProvider{})

	cfg := &config.ServerConfig{
		Reactors: []config.ReactorConfig{
			{
				Name:     "no-auth-reg",
				Match:    "true",
				Provider: "echo",
				Auth:     "some-handler",
				Inputs:   map[string]config.InputValue{},
			},
		},
	}

	a := New(cfg, m, reg, testLogger()) // no WithAuth call

	results := a.HandleEvent(context.Background(), message.Event{})
	require.Len(t, results, 1)
	// Should fail because auth is required but no registry is configured
	assert.Contains(t, results[0].Error, "no auth registry is configured")
}
