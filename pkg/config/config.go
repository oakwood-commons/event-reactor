// Package config handles loading and validating the event-reactor server
// configuration from YAML files.
package config

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ServerConfig is the root configuration for event-reactor.
type ServerConfig struct {
	APIVersion    string              `yaml:"apiVersion"    json:"apiVersion"`
	Kind          string              `yaml:"kind"          json:"kind"`
	Server        ServerSettings      `yaml:"server"        json:"server"`
	Observability ObservabilityConfig `yaml:"observability" json:"observability"`
	Auth          AuthConfig          `yaml:"auth"          json:"auth"`
	Listeners     []ListenerConfig    `yaml:"listeners"     json:"listeners"`
	Reactors      []ReactorConfig     `yaml:"reactors"      json:"reactors"`
}

// ServerSettings holds HTTP server configuration.
type ServerSettings struct {
	Port        int               `yaml:"port"        json:"port"`
	MetricsPort int               `yaml:"metricsPort" json:"metricsPort"`
	HealthCheck HealthCheckConfig `yaml:"healthCheck" json:"healthCheck"`
}

// HealthCheckConfig defines health endpoint paths.
type HealthCheckConfig struct {
	Liveness  string `yaml:"liveness"  json:"liveness"`
	Readiness string `yaml:"readiness" json:"readiness"`
}

// ObservabilityConfig holds logging, tracing, and metrics settings.
type ObservabilityConfig struct {
	Logging LoggingConfig `yaml:"logging" json:"logging"`
	Tracing TracingConfig `yaml:"tracing" json:"tracing"`
	Metrics MetricsConfig `yaml:"metrics" json:"metrics"`
}

// LoggingConfig controls structured logging.
type LoggingConfig struct {
	Level  string `yaml:"level"  json:"level"`
	Format string `yaml:"format" json:"format"`
}

// TracingConfig controls OpenTelemetry tracing.
type TracingConfig struct {
	Enabled    bool    `yaml:"enabled"    json:"enabled"`
	Exporter   string  `yaml:"exporter"   json:"exporter"`
	Endpoint   string  `yaml:"endpoint"   json:"endpoint"`
	SampleRate float64 `yaml:"sampleRate" json:"sampleRate"`
}

// MetricsConfig controls Prometheus metrics.
type MetricsConfig struct {
	Enabled    bool `yaml:"enabled"    json:"enabled"`
	Prometheus bool `yaml:"prometheus" json:"prometheus"`
}

// AuthConfig holds authentication configuration for inbound validation
// and outbound token generation.
type AuthConfig struct {
	// Handlers define named auth handlers for generating tokens on outbound calls.
	// Reactors reference these by name via the "auth" field.
	Handlers []AuthHandlerConfig `yaml:"handlers" json:"handlers"`

	// WebhookSecrets define HMAC secrets for validating inbound webhook signatures.
	WebhookSecrets []WebhookSecret `yaml:"webhookSecrets" json:"webhookSecrets"`
}

// WebhookSecret maps a webhook source to its HMAC secret.
type WebhookSecret struct {
	Source string `yaml:"source" json:"source"`
	Secret string `yaml:"secret" json:"secret"`
}

// AuthHandlerConfig defines a named auth handler for token generation.
// Supported types: github-app, github-token, oauth2-client-credentials,
// service-account, static-token.
type AuthHandlerConfig struct {
	Name          string         `yaml:"name"          json:"name"`
	Type          string         `yaml:"type"          json:"type"`
	DefaultScopes []string       `yaml:"defaultScopes" json:"defaultScopes"`
	Config        map[string]any `yaml:"config"        json:"config"`
}

// ListenerConfig defines an event source.
type ListenerConfig struct {
	Name   string         `yaml:"name"   json:"name"`
	Type   string         `yaml:"type"   json:"type"`
	Config map[string]any `yaml:"config" json:"config"`
}

// ReactorConfig defines a reactor that fires when an event matches.
type ReactorConfig struct {
	Name        string                `yaml:"name"        json:"name"`
	Match       string                `yaml:"match"       json:"match"`
	Provider    string                `yaml:"provider"    json:"provider"`
	Auth        string                `yaml:"auth"        json:"auth"`
	Disabled    bool                  `yaml:"disabled"    json:"disabled"`
	FailOnError *bool                 `yaml:"failOnError" json:"failOnError"`
	Inputs      map[string]InputValue `yaml:"inputs"      json:"inputs"`
	Retry       *RetryConfig          `yaml:"retry"       json:"retry"`
}

// GetFailOnError returns whether the reactor should fail the pipeline on error.
// Defaults to true when not explicitly set.
func (rc *ReactorConfig) GetFailOnError() bool {
	if rc.FailOnError == nil {
		return true
	}
	return *rc.FailOnError
}

// RetryConfig controls retry behavior for a reactor.
type RetryConfig struct {
	MaxAttempts int    `yaml:"maxAttempts" json:"maxAttempts"`
	Backoff     string `yaml:"backoff"     json:"backoff"`
}

// InputValue represents a reactor input that can be resolved from multiple sources.
// Resolution priority: secretKeyRef > payloadValue > fromFile > fromEnv > template > expr > static value.
type InputValue struct {
	value any
}

// inputValueYAML is the intermediate representation for complex input values.
type inputValueYAML struct {
	Template     *string          `yaml:"template"`
	Expr         *string          `yaml:"expr"`
	PayloadValue *PayloadValueRef `yaml:"payloadValue"`
	ValueFrom    *ValueFromRef    `yaml:"valueFrom"`
	FromFile     *string          `yaml:"fromFile"`
	FromEnv      *string          `yaml:"fromEnv"`
}

// PayloadValueRef extracts a value from the event payload via CEL paths.
type PayloadValueRef struct {
	PropertyPaths []string `yaml:"propertyPaths" json:"propertyPaths"`
}

// ValueFromRef references an external secret store.
type ValueFromRef struct {
	SecretKeyRef *SecretKeyRef `yaml:"secretKeyRef" json:"secretKeyRef"`
}

// SecretKeyRef references a GCP Secret Manager secret.
type SecretKeyRef struct {
	Name      string `yaml:"name"      json:"name"`
	ProjectID string `yaml:"projectId" json:"projectId"`
	Version   string `yaml:"version"   json:"version"`
	Path      string `yaml:"path"      json:"path"`
}

// UnmarshalYAML implements custom unmarshaling for InputValue.
// Simple scalar values (string, int, bool) are stored directly.
// Map values are parsed for special keys: template, expr, payloadValue, valueFrom, fromFile, fromEnv.
func (iv *InputValue) UnmarshalYAML(node *yaml.Node) error {
	// Try scalar first (string, int, bool, etc.)
	if node.Kind == yaml.ScalarNode {
		var s string
		if err := node.Decode(&s); err == nil {
			iv.value = s
			return nil
		}
	}

	// Try as complex input with special keys
	var cplx inputValueYAML
	if err := node.Decode(&cplx); err == nil {
		if cplx.Template != nil || cplx.Expr != nil ||
			cplx.PayloadValue != nil || cplx.ValueFrom != nil ||
			cplx.FromFile != nil || cplx.FromEnv != nil {
			iv.value = cplx
			return nil
		}
	}

	// Fall back to generic value (map, list, etc.)
	var generic any
	if err := node.Decode(&generic); err != nil {
		return fmt.Errorf("decoding input value: %w", err)
	}
	iv.value = generic
	return nil
}

// MarshalYAML implements custom marshaling for InputValue.
func (iv InputValue) MarshalYAML() (any, error) {
	return iv.value, nil
}

// IsStatic returns true if the value is a simple static value (not template/expr/ref).
func (iv InputValue) IsStatic() bool {
	_, ok := iv.value.(inputValueYAML)
	return !ok
}

// Static returns the raw static value, or nil if it's a complex input.
func (iv InputValue) Static() any {
	if iv.IsStatic() {
		return iv.value
	}
	return nil
}

// Template returns the Go template string, if set.
func (iv InputValue) Template() string {
	if c, ok := iv.value.(inputValueYAML); ok && c.Template != nil {
		return *c.Template
	}
	return ""
}

// Expr returns the CEL expression string, if set.
func (iv InputValue) Expr() string {
	if c, ok := iv.value.(inputValueYAML); ok && c.Expr != nil {
		return *c.Expr
	}
	return ""
}

// PayloadPaths returns the payload value property paths, if set.
func (iv InputValue) PayloadPaths() []string {
	if c, ok := iv.value.(inputValueYAML); ok && c.PayloadValue != nil {
		return c.PayloadValue.PropertyPaths
	}
	return nil
}

// SecretRef returns the secret key reference, if set.
func (iv InputValue) SecretRef() *SecretKeyRef {
	if c, ok := iv.value.(inputValueYAML); ok && c.ValueFrom != nil {
		return c.ValueFrom.SecretKeyRef
	}
	return nil
}

// FilePath returns the file path to read from, if set.
func (iv InputValue) FilePath() string {
	if c, ok := iv.value.(inputValueYAML); ok && c.FromFile != nil {
		return *c.FromFile
	}
	return ""
}

// EnvVar returns the environment variable name, if set.
func (iv InputValue) EnvVar() string {
	if c, ok := iv.value.(inputValueYAML); ok && c.FromEnv != nil {
		return *c.FromEnv
	}
	return ""
}

// NewInputStatic creates an InputValue with a static value.
func NewInputStatic(v any) InputValue {
	return InputValue{value: v}
}

// NewInputTemplate creates an InputValue with a Go template.
func NewInputTemplate(tmpl string) InputValue {
	return InputValue{value: inputValueYAML{Template: &tmpl}}
}

// NewInputExpr creates an InputValue with a CEL expression.
func NewInputExpr(expr string) InputValue {
	return InputValue{value: inputValueYAML{Expr: &expr}}
}

// Load reads and parses a server config from a YAML file.
func Load(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	return Parse(data)
}

// Parse parses a ServerConfig from YAML bytes.
func Parse(data []byte) (*ServerConfig, error) {
	cfg := &ServerConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// applyDefaults fills in sensible defaults for unset fields.
func applyDefaults(cfg *ServerConfig) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.MetricsPort == 0 {
		cfg.Server.MetricsPort = 9090
	}
	if cfg.Server.HealthCheck.Liveness == "" {
		cfg.Server.HealthCheck.Liveness = "/health/live"
	}
	if cfg.Server.HealthCheck.Readiness == "" {
		cfg.Server.HealthCheck.Readiness = "/health/ready"
	}
	if cfg.Observability.Logging.Level == "" {
		cfg.Observability.Logging.Level = "info"
	}
	if cfg.Observability.Logging.Format == "" {
		cfg.Observability.Logging.Format = "json"
	}
}

// validate checks that the config has all required fields.
func validate(cfg *ServerConfig) error {
	for i, r := range cfg.Reactors {
		if r.Name == "" {
			return fmt.Errorf("reactor[%d]: name is required", i)
		}
		if r.Provider == "" {
			return fmt.Errorf("reactor %q: provider is required", r.Name)
		}
		if r.Match == "" {
			return fmt.Errorf("reactor %q: match expression is required", r.Name)
		}
	}
	for i, l := range cfg.Listeners {
		if l.Name == "" {
			return fmt.Errorf("listener[%d]: name is required", i)
		}
		if l.Type == "" {
			return fmt.Errorf("listener %q: type is required", l.Name)
		}
	}
	return nil
}

type ctxConfigKey struct{}

// FromContext returns the ServerConfig from the context, or nil.
func FromContext(ctx context.Context) *ServerConfig {
	cfg, _ := ctx.Value(ctxConfigKey{}).(*ServerConfig)
	return cfg
}

// WithContext stores a ServerConfig in the context.
func WithContext(ctx context.Context, cfg *ServerConfig) context.Context {
	return context.WithValue(ctx, ctxConfigKey{}, cfg)
}
