// Package reactor dispatches matched events to providers.
// It resolves reactor inputs (static, template, CEL, secret, env, file)
// and calls the registered provider with the resolved values.
package reactor

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/oakwood-commons/event-reactor/pkg/config"
	"github.com/oakwood-commons/event-reactor/pkg/matcher"
	"github.com/oakwood-commons/event-reactor/pkg/message"
	ertmpl "github.com/oakwood-commons/event-reactor/pkg/template"
)

// authHeaderKey is the context key for auth header injection.
type authHeaderKey struct{}

// WithAuthHeader returns a context carrying the Authorization header value.
func WithAuthHeader(ctx context.Context, header string) context.Context {
	return context.WithValue(ctx, authHeaderKey{}, header)
}

// AuthHeader extracts the Authorization header from context, if present.
func AuthHeader(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(authHeaderKey{}).(string)
	return v, ok && v != ""
}

// Provider executes a reaction with the given resolved inputs.
// Implementations wrap scafctl providers, HTTP calls, shell commands, etc.
type Provider interface {
	// Name returns the provider identifier (e.g., "github", "http", "exec").
	Name() string

	// Execute runs the provider with resolved inputs against the event.
	Execute(ctx context.Context, inputs map[string]any, event message.Event) (*Result, error)
}

// Result holds the output of a provider execution.
type Result struct {
	// Provider is the name of the provider that executed.
	Provider string `json:"provider"`

	// ReactorName is the name of the reactor config that triggered this.
	ReactorName string `json:"reactorName"`

	// Output is the provider-specific output data.
	Output any `json:"output,omitempty"`

	// Error is set if the provider returned an error.
	Error string `json:"error,omitempty"`
}

// Registry holds named provider implementations.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider to the registry.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	r.providers[p.Name()] = p
	r.mu.Unlock()
}

// Get returns the provider with the given name, or an error if not found.
func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	p, ok := r.providers[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", name)
	}
	return p, nil
}

// Providers returns the names of all registered providers.
func (r *Registry) Providers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ResolveInputs resolves all InputValue entries in a reactor config against
// the given event. Secret resolution is not yet implemented.
func ResolveInputs(cfg config.ReactorConfig, event message.Event, m *matcher.Matcher) (map[string]any, error) {
	resolved := make(map[string]any, len(cfg.Inputs))

	for key, iv := range cfg.Inputs {
		val, err := resolveInput(iv, event, m)
		if err != nil {
			return nil, fmt.Errorf("resolving input %q for reactor %q: %w", key, cfg.Name, err)
		}
		resolved[key] = val
	}

	return resolved, nil
}

// resolveInput resolves a single InputValue against the event.
// Resolution priority: secretKeyRef > payloadValue > fromFile > fromEnv > expr > template > static.
func resolveInput(iv config.InputValue, event message.Event, m *matcher.Matcher) (any, error) {
	// Secret refs (stub -- requires auth handler integration)
	if ref := iv.SecretRef(); ref != nil {
		return nil, fmt.Errorf("secret resolution not yet implemented (secret: %s/%s)", ref.ProjectID, ref.Name)
	}

	// Payload value paths -- evaluate CEL paths, first match wins
	if paths := iv.PayloadPaths(); len(paths) > 0 {
		data := event.AsMap()
		for _, path := range paths {
			val, err := evalCELValue(path, m, data)
			if err == nil {
				return val, nil
			}
		}
		return nil, fmt.Errorf("no payload path matched from %v", paths)
	}

	// File content
	if path := iv.FilePath(); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading file %s: %w", path, err)
		}
		return string(data), nil
	}

	// Environment variable
	if envVar := iv.EnvVar(); envVar != "" {
		return os.Getenv(envVar), nil
	}

	// CEL expression -- evaluate and return result
	if expr := iv.Expr(); expr != "" {
		return evalCELValue(expr, m, event.AsMap())
	}

	// Go template -- render against event data
	if tmpl := iv.Template(); tmpl != "" {
		return ertmpl.Render(tmpl, event.AsMap())
	}

	// Static value
	return iv.Static(), nil
}

// evalCELValue compiles and evaluates a CEL expression, returning the native Go value.
func evalCELValue(expr string, m *matcher.Matcher, data map[string]any) (any, error) {
	prg, err := m.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("compiling CEL expression %q: %w", expr, err)
	}

	out, _, err := prg.Eval(data)
	if err != nil {
		return nil, fmt.Errorf("evaluating CEL expression %q: %w", expr, err)
	}

	return out.Value(), nil
}
