package mcp

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oakwood-commons/event-reactor/pkg/message"
	"github.com/oakwood-commons/event-reactor/pkg/reactor"
)

type fakeProvider struct {
	name string
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Execute(_ context.Context, inputs map[string]any, _ message.Event) (*reactor.Result, error) {
	return &reactor.Result{Provider: f.name, Output: inputs}, nil
}

func testServer(t *testing.T) *Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	reg := reactor.NewRegistry()
	return New(reg, logger)
}

func TestListTools(t *testing.T) {
	s := testServer(t)
	tools := s.ListTools()
	assert.Len(t, tools, 6)

	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	assert.Contains(t, names, "list_providers")
	assert.Contains(t, names, "validate_config")
	assert.Contains(t, names, "test_cel_expression")
	assert.Contains(t, names, "render_template")
	assert.Contains(t, names, "test_reactor")
	assert.Contains(t, names, "list_event_sources")
}

func TestCallTool_UnknownTool(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "nonexistent", nil)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "unknown tool")
}

func TestListProviders_Empty(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "list_providers", nil)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "No providers registered")
}

func TestValidateConfig_Valid(t *testing.T) {
	s := testServer(t)
	cfg := `
reactors:
  - name: test
    match: "true"
    provider: http
    inputs:
      method: POST
`
	result := s.CallTool(context.Background(), "validate_config", map[string]any{"config": cfg})
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "Config valid")
}

func TestValidateConfig_Invalid(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "validate_config", map[string]any{
		"config": `reactors: [{name: test}]`,
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "invalid config")
}

func TestTestCELExpression_Match(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "test_cel_expression", map[string]any{
		"expression": `payload.action == "opened"`,
		"event":      map[string]any{"action": "opened"},
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "MATCH")
}

func TestTestCELExpression_NoMatch(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "test_cel_expression", map[string]any{
		"expression": `payload.action == "closed"`,
		"event":      map[string]any{"action": "opened"},
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "NO MATCH")
}

func TestRenderTemplate(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "render_template", map[string]any{
		"template": "Hello {{ .payload.name }}!",
		"event":    map[string]any{"name": "World"},
	})
	assert.False(t, result.IsError)
	assert.Equal(t, "Hello World!", result.Content[0].Text)
}

func TestTestReactor_Match(t *testing.T) {
	s := testServer(t)
	cfg := `
reactors:
  - name: test-reactor
    match: "true"
    provider: echo
    inputs:
      msg: hello
`
	result := s.CallTool(context.Background(), "test_reactor", map[string]any{
		"config":  cfg,
		"reactor": "test-reactor",
		"event":   map[string]any{"action": "test"},
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "test-reactor")
	assert.Contains(t, result.Content[0].Text, `"matched": true`)
}

func TestTestReactor_NotFound(t *testing.T) {
	s := testServer(t)
	cfg := `
reactors:
  - name: test
    match: "true"
    provider: echo
    inputs:
      msg: hello
`
	result := s.CallTool(context.Background(), "test_reactor", map[string]any{
		"config":  cfg,
		"reactor": "missing",
		"event":   map[string]any{},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "not found")
}

func TestListEventSources(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "list_event_sources", nil)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "pubsub")
	assert.Contains(t, result.Content[0].Text, "cloudevents")
	assert.Contains(t, result.Content[0].Text, "webhook")
}

func TestValidateConfig_MissingArg(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "validate_config", map[string]any{
		"config": 123, // not a string
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "config must be a string")
}

func TestValidateConfig_InvalidCEL(t *testing.T) {
	s := testServer(t)
	cfg := `
reactors:
  - name: test
    match: "invalid cel %%"
    provider: echo
    inputs:
      msg: hello
`
	result := s.CallTool(context.Background(), "validate_config", map[string]any{"config": cfg})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "invalid CEL expression")
}

func TestTestCELExpression_MissingExpression(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "test_cel_expression", map[string]any{
		"expression": 42,
		"event":      map[string]any{},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "expression must be a string")
}

func TestTestCELExpression_MissingEvent(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "test_cel_expression", map[string]any{
		"expression": "true",
		"event":      "not-a-map",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "event must be a JSON object")
}

func TestTestCELExpression_InvalidExpr(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "test_cel_expression", map[string]any{
		"expression": "invalid %%",
		"event":      map[string]any{},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "expression error")
}

func TestRenderTemplate_MissingTemplate(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "render_template", map[string]any{
		"template": 123,
		"event":    map[string]any{},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "template must be a string")
}

func TestRenderTemplate_MissingEvent(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "render_template", map[string]any{
		"template": "{{ .payload.action }}",
		"event":    "not-a-map",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "event must be a JSON object")
}

func TestRenderTemplate_InvalidTemplate(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "render_template", map[string]any{
		"template": "{{ .payload.action",
		"event":    map[string]any{},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "template error")
}

func TestTestReactor_MissingConfig(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "test_reactor", map[string]any{
		"config":  123,
		"reactor": "test",
		"event":   map[string]any{},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "config must be a string")
}

func TestTestReactor_MissingReactor(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "test_reactor", map[string]any{
		"config":  "reactors: []",
		"reactor": 123,
		"event":   map[string]any{},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "reactor must be a string")
}

func TestTestReactor_MissingEvent(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "test_reactor", map[string]any{
		"config":  "reactors: []",
		"reactor": "test",
		"event":   "not-a-map",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "event must be a JSON object")
}

func TestTestReactor_InvalidConfig(t *testing.T) {
	s := testServer(t)
	result := s.CallTool(context.Background(), "test_reactor", map[string]any{
		"config":  "invalid yaml: [",
		"reactor": "test",
		"event":   map[string]any{},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "invalid config")
}

func TestTestReactor_NoMatch(t *testing.T) {
	s := testServer(t)
	cfg := `
reactors:
  - name: test-reactor
    match: "false"
    provider: echo
    inputs:
      msg: hello
`
	result := s.CallTool(context.Background(), "test_reactor", map[string]any{
		"config":  cfg,
		"reactor": "test-reactor",
		"event":   map[string]any{"action": "test"},
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "NO MATCH")
}

func TestTestReactor_MatchError(t *testing.T) {
	s := testServer(t)
	cfg := `
reactors:
  - name: test-reactor
    match: "invalid cel %%"
    provider: echo
    inputs:
      msg: hello
`
	result := s.CallTool(context.Background(), "test_reactor", map[string]any{
		"config":  cfg,
		"reactor": "test-reactor",
		"event":   map[string]any{"action": "test"},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "match error")
}

func TestListProviders_WithProviders(t *testing.T) {
	s := testServer(t)
	s.registry.Register(&fakeProvider{name: "echo"})
	s.registry.Register(&fakeProvider{name: "http"})

	result := s.CallTool(context.Background(), "list_providers", nil)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "echo")
	assert.Contains(t, result.Content[0].Text, "http")
}
