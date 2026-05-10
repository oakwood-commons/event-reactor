package reactor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oakwood-commons/event-reactor/pkg/config"
	"github.com/oakwood-commons/event-reactor/pkg/matcher"
	"github.com/oakwood-commons/event-reactor/pkg/message"
)

func testMatcher(t *testing.T) *matcher.Matcher {
	t.Helper()
	m, err := matcher.New()
	require.NoError(t, err)
	return m
}

// fakeProvider is a test double for Provider.
type fakeProvider struct {
	name   string
	output any
	err    error
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Execute(_ context.Context, inputs map[string]any, _ message.Event) (*Result, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &Result{
		Provider: f.name,
		Output:   inputs,
	}, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	p := &fakeProvider{name: "http"}
	r.Register(p)

	got, err := r.Get("http")
	require.NoError(t, err)
	assert.Equal(t, "http", got.Name())
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `provider "missing" not registered`)
}

func TestRegistry_Providers(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeProvider{name: "http"})
	r.Register(&fakeProvider{name: "github"})

	names := r.Providers()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "http")
	assert.Contains(t, names, "github")
}

func TestResolveInputs_Static(t *testing.T) {
	cfg := config.ReactorConfig{
		Name:     "test",
		Provider: "http",
		Match:    "true",
		Inputs: map[string]config.InputValue{
			"method": config.NewInputStatic("POST"),
			"url":    config.NewInputStatic("https://example.com"),
		},
	}

	resolved, err := ResolveInputs(cfg, message.Event{}, testMatcher(t))
	require.NoError(t, err)
	assert.Equal(t, "POST", resolved["method"])
	assert.Equal(t, "https://example.com", resolved["url"])
}

func TestResolveInputs_FromEnv(t *testing.T) {
	t.Setenv("TEST_REACTOR_TOKEN", "abc123")

	cfg := config.ReactorConfig{
		Name:     "test",
		Provider: "http",
		Match:    "true",
		Inputs: map[string]config.InputValue{
			"token": config.NewInputStatic(nil), // will be replaced below
		},
	}

	// Build via YAML parsing to get fromEnv
	yamlStr := `
reactors:
  - name: test
    match: "true"
    provider: http
    inputs:
      token:
        fromEnv: TEST_REACTOR_TOKEN
`
	parsed, err := config.Parse([]byte(yamlStr))
	require.NoError(t, err)

	resolved, err := ResolveInputs(parsed.Reactors[0], message.Event{}, testMatcher(t))
	require.NoError(t, err)
	assert.Equal(t, "abc123", resolved["token"])
	_ = cfg // suppress unused
}

func TestResolveInputs_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("file-content"), 0o600))

	yamlStr := `
reactors:
  - name: test
    match: "true"
    provider: http
    inputs:
      data:
        fromFile: ` + path + `
`
	parsed, err := config.Parse([]byte(yamlStr))
	require.NoError(t, err)

	resolved, err := ResolveInputs(parsed.Reactors[0], message.Event{}, testMatcher(t))
	require.NoError(t, err)
	assert.Equal(t, "file-content", resolved["data"])
}

func TestResolveInputs_SecretRef_NotImplemented(t *testing.T) {
	yamlStr := `
reactors:
  - name: test
    match: "true"
    provider: http
    inputs:
      key:
        valueFrom:
          secretKeyRef:
            name: my-secret
            projectId: my-project
            version: latest
`
	parsed, err := config.Parse([]byte(yamlStr))
	require.NoError(t, err)

	_, err = ResolveInputs(parsed.Reactors[0], message.Event{}, testMatcher(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secret resolution not yet implemented")
}

func TestResolveInputs_Template(t *testing.T) {
	cfg := config.ReactorConfig{
		Name:     "test",
		Provider: "http",
		Match:    "true",
		Inputs: map[string]config.InputValue{
			"body": config.NewInputTemplate("{{ .payload.action }}"),
		},
	}

	ev := message.Event{
		Payload: map[string]any{"action": "opened"},
	}
	resolved, err := ResolveInputs(cfg, ev, testMatcher(t))
	require.NoError(t, err)
	assert.Equal(t, "opened", resolved["body"])
}

func TestResolveInputs_Expr(t *testing.T) {
	cfg := config.ReactorConfig{
		Name:     "test",
		Provider: "http",
		Match:    "true",
		Inputs: map[string]config.InputValue{
			"count": config.NewInputExpr(`2 + 3`),
		},
	}

	resolved, err := ResolveInputs(cfg, message.Event{}, testMatcher(t))
	require.NoError(t, err)
	assert.Equal(t, int64(5), resolved["count"])
}

func TestResolveInputs_PayloadValue(t *testing.T) {
	yamlStr := `
reactors:
  - name: test
    match: "true"
    provider: http
    inputs:
      action:
        payloadValue:
          propertyPaths:
            - payload.action
`
	parsed, err := config.Parse([]byte(yamlStr))
	require.NoError(t, err)

	ev := message.Event{
		Payload: map[string]any{"action": "opened"},
	}
	resolved, err := ResolveInputs(parsed.Reactors[0], ev, testMatcher(t))
	require.NoError(t, err)
	assert.Equal(t, "opened", resolved["action"])
}
