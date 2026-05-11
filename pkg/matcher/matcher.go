// Package matcher evaluates CEL expressions against event envelopes
// to determine which reactors should fire for a given event.
package matcher

import (
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"

	"github.com/oakwood-commons/event-reactor/pkg/message"
)

// celEnvOptions defines the CEL environment for event matching.
// Variables available in expressions: payload, attributes, id, source, type.
func celEnvOptions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Variable("payload", cel.DynType),
		cel.Variable("attributes", cel.MapType(cel.StringType, cel.StringType)),
		cel.Variable("id", cel.StringType),
		cel.Variable("source", cel.StringType),
		cel.Variable("type", cel.StringType),
	}
}

// Matcher compiles and caches CEL programs for event filtering.
type Matcher struct {
	mu    sync.RWMutex
	cache map[string]cel.Program
	env   *cel.Env
}

// New creates a Matcher with a shared CEL environment.
func New() (*Matcher, error) {
	env, err := cel.NewEnv(celEnvOptions()...)
	if err != nil {
		return nil, fmt.Errorf("creating CEL environment: %w", err)
	}
	return &Matcher{
		cache: make(map[string]cel.Program),
		env:   env,
	}, nil
}

// Compile parses and type-checks a CEL expression, caching the result.
// Returns an error if the expression is invalid.
func (m *Matcher) Compile(expr string) (cel.Program, error) {
	m.mu.RLock()
	if prg, ok := m.cache[expr]; ok {
		m.mu.RUnlock()
		return prg, nil
	}
	m.mu.RUnlock()

	ast, issues := m.env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compiling CEL expression %q: %w", expr, issues.Err())
	}

	prg, err := m.env.Program(ast, cel.EvalOptions(cel.OptOptimize))
	if err != nil {
		return nil, fmt.Errorf("creating CEL program for %q: %w", expr, err)
	}

	m.mu.Lock()
	m.cache[expr] = prg
	m.mu.Unlock()

	return prg, nil
}

// Match evaluates a CEL expression against an event and returns whether it matched.
// Empty expressions match all events. Invalid expressions return an error.
func (m *Matcher) Match(expr string, event message.Event) (bool, error) {
	if expr == "" {
		return true, nil
	}

	prg, err := m.Compile(expr)
	if err != nil {
		return false, err
	}

	out, _, err := prg.Eval(event.AsMap())
	if err != nil {
		return false, fmt.Errorf("evaluating CEL expression %q: %w", expr, err)
	}

	return out == types.True, nil
}
