// Package adapter bridges listeners to the matcher/reactor pipeline.
// It receives events from listeners, fans them out to matching reactors,
// and manages the concurrent execution of reactions.
package adapter

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/oakwood-commons/event-reactor/pkg/auth"
	"github.com/oakwood-commons/event-reactor/pkg/config"
	"github.com/oakwood-commons/event-reactor/pkg/matcher"
	"github.com/oakwood-commons/event-reactor/pkg/message"
	"github.com/oakwood-commons/event-reactor/pkg/reactor"
)

// Adapter receives events and dispatches them to matching reactors.
type Adapter struct {
	cfg      *config.ServerConfig
	matcher  *matcher.Matcher
	registry *reactor.Registry
	auth     *auth.Registry
	logger   *slog.Logger
}

// New creates an Adapter with the given config, matcher, and provider registry.
func New(cfg *config.ServerConfig, m *matcher.Matcher, r *reactor.Registry, logger *slog.Logger) *Adapter {
	return &Adapter{
		cfg:      cfg,
		matcher:  m,
		registry: r,
		logger:   logger,
	}
}

// WithAuth sets the auth registry for token injection.
func (a *Adapter) WithAuth(ar *auth.Registry) *Adapter {
	a.auth = ar
	return a
}

// HandleEvent evaluates all reactor configs against the event and dispatches
// matching reactors concurrently. Returns results for all matched reactors.
func (a *Adapter) HandleEvent(ctx context.Context, event message.Event) []reactor.Result {
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results []reactor.Result
	)

	for _, rc := range a.cfg.Reactors {
		if rc.Disabled {
			a.logger.DebugContext(ctx, "reactor disabled, skipping",
				slog.String("reactor", rc.Name))
			continue
		}

		matched, err := a.matcher.Match(rc.Match, event)
		if err != nil {
			a.logger.ErrorContext(ctx, "error evaluating match expression",
				slog.String("reactor", rc.Name),
				slog.String("error", err.Error()))
			continue
		}
		if !matched {
			a.logger.DebugContext(ctx, "event did not match reactor",
				slog.String("reactor", rc.Name))
			continue
		}

		wg.Add(1)
		go func(rc config.ReactorConfig) {
			defer wg.Done()
			result := a.executeReactor(ctx, rc, event)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(rc)
	}

	wg.Wait()
	return results
}

// executeReactor resolves inputs and calls the provider for a single reactor.
func (a *Adapter) executeReactor(ctx context.Context, rc config.ReactorConfig, event message.Event) reactor.Result {
	log := a.logger.With(
		slog.String("reactor", rc.Name),
		slog.String("provider", rc.Provider),
	)

	// Resolve inputs
	inputs, err := reactor.ResolveInputs(rc, event, a.matcher)
	if err != nil {
		log.ErrorContext(ctx, "failed to resolve inputs", slog.String("error", err.Error()))
		return reactor.Result{
			Provider:    rc.Provider,
			ReactorName: rc.Name,
			Error:       fmt.Sprintf("resolving inputs: %v", err),
		}
	}

	// Inject auth token if the reactor references an auth handler
	if rc.Auth != "" {
		if a.auth == nil {
			log.ErrorContext(ctx, "reactor requires auth but no auth registry configured",
				slog.String("auth", rc.Auth))
			return reactor.Result{
				Provider:    rc.Provider,
				ReactorName: rc.Name,
				Error:       fmt.Sprintf("reactor %q requires auth handler %q but no auth registry is configured", rc.Name, rc.Auth),
			}
		}
		tok, err := a.auth.GetToken(ctx, rc.Auth)
		if err != nil {
			log.ErrorContext(ctx, "failed to resolve auth token",
				slog.String("auth", rc.Auth), slog.String("error", err.Error()))
			return reactor.Result{
				Provider:    rc.Provider,
				ReactorName: rc.Name,
				Error:       fmt.Sprintf("auth token: %v", err),
			}
		}
		ctx = reactor.WithAuthHeader(ctx, tok.Header())
	}

	// Look up provider
	p, err := a.registry.Get(rc.Provider)
	if err != nil {
		log.ErrorContext(ctx, "provider not found", slog.String("error", err.Error()))
		return reactor.Result{
			Provider:    rc.Provider,
			ReactorName: rc.Name,
			Error:       fmt.Sprintf("provider lookup: %v", err),
		}
	}

	// Execute
	result, err := p.Execute(ctx, inputs, event)
	if err != nil {
		log.ErrorContext(ctx, "provider execution failed", slog.String("error", err.Error()))
		return reactor.Result{
			Provider:    rc.Provider,
			ReactorName: rc.Name,
			Error:       fmt.Sprintf("execution: %v", err),
		}
	}

	result.ReactorName = rc.Name
	log.InfoContext(ctx, "reactor executed successfully")
	return *result
}
