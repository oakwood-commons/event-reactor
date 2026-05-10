package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestLoad_FullConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "full-config.yaml"))
	require.NoError(t, err)

	assert.Equal(t, "event-reactor.io/v1", cfg.APIVersion)
	assert.Equal(t, "ServerConfig", cfg.Kind)

	// Server
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 9090, cfg.Server.MetricsPort)
	assert.Equal(t, "/health/live", cfg.Server.HealthCheck.Liveness)
	assert.Equal(t, "/health/ready", cfg.Server.HealthCheck.Readiness)

	// Observability
	assert.Equal(t, "info", cfg.Observability.Logging.Level)
	assert.Equal(t, "json", cfg.Observability.Logging.Format)
	assert.True(t, cfg.Observability.Tracing.Enabled)
	assert.Equal(t, "otlp", cfg.Observability.Tracing.Exporter)
	assert.Equal(t, 0.1, cfg.Observability.Tracing.SampleRate)
	assert.True(t, cfg.Observability.Metrics.Enabled)
	assert.True(t, cfg.Observability.Metrics.Prometheus)

	// Auth
	require.Len(t, cfg.Auth.Handlers, 2)
	assert.Equal(t, "gcp", cfg.Auth.Handlers[0].Name)
	assert.Equal(t, "github-app", cfg.Auth.Handlers[1].Type)

	// Listeners
	require.Len(t, cfg.Listeners, 2)
	assert.Equal(t, "gcp-pubsub-events", cfg.Listeners[0].Name)
	assert.Equal(t, "pubsub", cfg.Listeners[0].Type)
	assert.Equal(t, "my-project", cfg.Listeners[0].Config["projectId"])
	assert.Equal(t, "webhook", cfg.Listeners[1].Type)

	// Reactors
	require.Len(t, cfg.Reactors, 3)

	// Reactor 0: pr-review-checklist
	r0 := cfg.Reactors[0]
	assert.Equal(t, "pr-review-checklist", r0.Name)
	assert.Equal(t, "github", r0.Provider)
	assert.Equal(t, "github", r0.Auth)
	assert.False(t, r0.GetFailOnError())
	assert.Equal(t, "create-comment", r0.Inputs["operation"].Static())
	assert.Contains(t, r0.Inputs["owner"].Template(), ".payload.repository.owner.login")
	assert.Contains(t, r0.Inputs["body"].Template(), "Review Checklist")

	// Reactor 1: error-to-slack
	r1 := cfg.Reactors[1]
	assert.Equal(t, "http", r1.Provider)
	assert.Equal(t, "POST", r1.Inputs["method"].Static())
	assert.Contains(t, r1.Inputs["headers"].Expr(), "Content-Type")

	// Reactor 2: rotate-secrets
	r2 := cfg.Reactors[2]
	assert.Equal(t, "exec", r2.Provider)
	require.NotNil(t, r2.Retry)
	assert.Equal(t, 3, r2.Retry.MaxAttempts)
	assert.Equal(t, "exponential", r2.Retry.Backoff)

	// payloadValue
	assert.Equal(t, []string{"attributes.secretName"}, r2.Inputs["secretName"].PayloadPaths())

	// secretKeyRef
	ref := r2.Inputs["rotationKey"].SecretRef()
	require.NotNil(t, ref)
	assert.Equal(t, "rotation-keys", ref.Name)
	assert.Equal(t, "my-project", ref.ProjectID)
	assert.Equal(t, "latest", ref.Version)
	assert.Equal(t, "keys.default", ref.Path)

	// fromEnv
	assert.Equal(t, "GCP_PROJECT", r2.Inputs["gcpProject"].EnvVar())

	// fromFile
	assert.Equal(t, "/etc/config/rotate-config.json", r2.Inputs["scriptConfig"].FilePath())
}

func TestLoad_MinimalConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "minimal-config.yaml"))
	require.NoError(t, err)

	// Defaults applied
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 9090, cfg.Server.MetricsPort)
	assert.Equal(t, "/health/live", cfg.Server.HealthCheck.Liveness)
	assert.Equal(t, "info", cfg.Observability.Logging.Level)
	assert.Equal(t, "json", cfg.Observability.Logging.Format)

	require.Len(t, cfg.Listeners, 1)
	require.Len(t, cfg.Reactors, 1)
	assert.True(t, cfg.Reactors[0].GetFailOnError()) // default true
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("testdata/does-not-exist.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := Parse([]byte("{{invalid yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config")
}

func TestParse_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "reactor missing name",
			yaml: `
reactors:
  - match: "true"
    provider: http
`,
			wantErr: "reactor[0]: name is required",
		},
		{
			name: "reactor missing provider",
			yaml: `
reactors:
  - name: test
    match: "true"
`,
			wantErr: `reactor "test": provider is required`,
		},
		{
			name: "reactor missing match",
			yaml: `
reactors:
  - name: test
    provider: http
`,
			wantErr: `reactor "test": match expression is required`,
		},
		{
			name: "listener missing name",
			yaml: `
listeners:
  - type: pubsub
`,
			wantErr: "listener[0]: name is required",
		},
		{
			name: "listener missing type",
			yaml: `
listeners:
  - name: test
`,
			wantErr: `listener "test": type is required`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestInputValue_StaticTypes(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want any
	}{
		{name: "string", yaml: "value: hello", want: "hello"},
		{name: "int", yaml: "value: 42", want: "42"},
		{name: "bool", yaml: "value: true", want: "true"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var m struct {
				Value InputValue `yaml:"value"`
			}
			require.NoError(t, unmarshalYAML(tc.yaml, &m))
			assert.True(t, m.Value.IsStatic())
			assert.Equal(t, tc.want, m.Value.Static())
		})
	}
}

func TestInputValue_ComplexTypes(t *testing.T) {
	t.Run("template", func(t *testing.T) {
		var m struct {
			Value InputValue `yaml:"value"`
		}
		require.NoError(t, unmarshalYAML(`value: {template: "{{ .name }}"}`, &m))
		assert.False(t, m.Value.IsStatic())
		assert.Equal(t, "{{ .name }}", m.Value.Template())
		assert.Nil(t, m.Value.Static())
	})

	t.Run("expr", func(t *testing.T) {
		var m struct {
			Value InputValue `yaml:"value"`
		}
		require.NoError(t, unmarshalYAML(`value: {expr: "a + b"}`, &m))
		assert.Equal(t, "a + b", m.Value.Expr())
	})

	t.Run("fromEnv", func(t *testing.T) {
		var m struct {
			Value InputValue `yaml:"value"`
		}
		require.NoError(t, unmarshalYAML(`value: {fromEnv: MY_VAR}`, &m))
		assert.Equal(t, "MY_VAR", m.Value.EnvVar())
	})

	t.Run("fromFile", func(t *testing.T) {
		var m struct {
			Value InputValue `yaml:"value"`
		}
		require.NoError(t, unmarshalYAML(`value: {fromFile: /tmp/data.json}`, &m))
		assert.Equal(t, "/tmp/data.json", m.Value.FilePath())
	})
}

func TestNewInputHelpers(t *testing.T) {
	s := NewInputStatic("hello")
	assert.True(t, s.IsStatic())
	assert.Equal(t, "hello", s.Static())

	tmpl := NewInputTemplate("{{ .name }}")
	assert.False(t, tmpl.IsStatic())
	assert.Equal(t, "{{ .name }}", tmpl.Template())

	expr := NewInputExpr("a + b")
	assert.False(t, expr.IsStatic())
	assert.Equal(t, "a + b", expr.Expr())
}

func TestReactorConfig_GetFailOnError(t *testing.T) {
	t.Run("nil defaults to true", func(t *testing.T) {
		rc := ReactorConfig{}
		assert.True(t, rc.GetFailOnError())
	})

	t.Run("explicit false", func(t *testing.T) {
		f := false
		rc := ReactorConfig{FailOnError: &f}
		assert.False(t, rc.GetFailOnError())
	})

	t.Run("explicit true", func(t *testing.T) {
		tr := true
		rc := ReactorConfig{FailOnError: &tr}
		assert.True(t, rc.GetFailOnError())
	})
}

func TestContextRoundTrip(t *testing.T) {
	cfg := &ServerConfig{APIVersion: "v1"}
	ctx := WithContext(context.Background(), cfg)
	got := FromContext(ctx)
	assert.Equal(t, cfg, got)
}

func TestFromContext_Empty(t *testing.T) {
	got := FromContext(context.Background())
	assert.Nil(t, got)
}

func TestLoad_FromEnvResolution(t *testing.T) {
	yaml := `
reactors:
  - name: test
    match: "true"
    provider: http
    inputs:
      token:
        fromEnv: TEST_TOKEN_VAR
`
	t.Setenv("TEST_TOKEN_VAR", "secret-value")
	cfg, err := Parse([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, "TEST_TOKEN_VAR", cfg.Reactors[0].Inputs["token"].EnvVar())

	// Verify env resolution would work
	val := os.Getenv(cfg.Reactors[0].Inputs["token"].EnvVar())
	assert.Equal(t, "secret-value", val)
}

func unmarshalYAML(s string, v any) error {
	return yaml.Unmarshal([]byte(s), v)
}
