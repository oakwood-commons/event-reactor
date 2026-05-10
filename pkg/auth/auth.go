// Package auth resolves authentication tokens for outbound reactor calls.
// It supports multiple handler types: static tokens, GitHub App installations,
// GitHub PATs, OAuth2 client credentials, and GCP service accounts.
package auth

import (
	"context"
	"fmt"
	"sync"

	"github.com/oakwood-commons/event-reactor/pkg/config"
)

// configStr extracts a string value from the handler config map.
// Non-string values are converted via fmt.Sprint.
func configStr(cfg map[string]any, key string) string {
	v, ok := cfg[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// Token represents a resolved authentication token.
type Token struct {
	// Value is the bearer token or credential.
	Value string

	// Type is the token type (e.g., "Bearer", "token", "Basic").
	Type string
}

// Header returns the Authorization header value.
func (t Token) Header() string {
	if t.Type == "" {
		return "Bearer " + t.Value
	}
	return t.Type + " " + t.Value
}

// Handler generates tokens for a specific auth configuration.
type Handler interface {
	// Name returns the handler name (matches config name).
	Name() string

	// GetToken returns a valid token, refreshing if needed.
	GetToken(ctx context.Context) (*Token, error)
}

// Registry manages named auth handlers.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

// NewRegistry creates an auth handler registry from config.
func NewRegistry(handlers []config.AuthHandlerConfig) (*Registry, error) {
	r := &Registry{handlers: make(map[string]Handler)}
	for _, h := range handlers {
		handler, err := newHandler(h)
		if err != nil {
			return nil, fmt.Errorf("auth handler %q: %w", h.Name, err)
		}
		r.handlers[h.Name] = handler
	}
	return r, nil
}

// GetToken resolves a token from the named handler.
func (r *Registry) GetToken(ctx context.Context, name string) (*Token, error) {
	r.mu.RLock()
	h, ok := r.handlers[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("auth handler %q not found", name)
	}
	return h.GetToken(ctx)
}

// Has returns true if a handler with the given name exists.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.handlers[name]
	return ok
}

func newHandler(cfg config.AuthHandlerConfig) (Handler, error) {
	switch cfg.Type {
	case "static-token":
		return newStaticToken(cfg)
	case "github-token":
		return newGitHubToken(cfg)
	case "github-app":
		return newGitHubApp(cfg)
	case "oauth2-client-credentials":
		return newOAuth2ClientCredentials(cfg)
	case "service-account":
		return newServiceAccount(cfg)
	default:
		return nil, fmt.Errorf("unsupported auth handler type: %s", cfg.Type)
	}
}

// staticToken is a simple bearer token handler.
type staticToken struct {
	name  string
	token string
	typ   string
}

func newStaticToken(cfg config.AuthHandlerConfig) (*staticToken, error) {
	tok := configStr(cfg.Config, "token")
	if tok == "" {
		return nil, fmt.Errorf("static-token: token is required")
	}
	typ := configStr(cfg.Config, "tokenType")
	if typ == "" {
		typ = "Bearer"
	}
	return &staticToken{name: cfg.Name, token: tok, typ: typ}, nil
}

func (s *staticToken) Name() string { return s.name }

func (s *staticToken) GetToken(_ context.Context) (*Token, error) {
	return &Token{Value: s.token, Type: s.typ}, nil
}

// githubToken wraps a GitHub PAT.
type githubToken struct {
	name  string
	token string
}

func newGitHubToken(cfg config.AuthHandlerConfig) (*githubToken, error) {
	tok := configStr(cfg.Config, "token")
	if tok == "" {
		return nil, fmt.Errorf("github-token: token is required")
	}
	return &githubToken{name: cfg.Name, token: tok}, nil
}

func (g *githubToken) Name() string { return g.name }

func (g *githubToken) GetToken(_ context.Context) (*Token, error) {
	return &Token{Value: g.token, Type: "token"}, nil
}

// githubApp generates installation tokens from a GitHub App.
// Token refresh is not yet implemented -- this is a placeholder for the
// JWT -> installation token exchange flow.
type githubApp struct {
	name           string
	appID          string
	installationID string
	privateKeyPath string
}

func newGitHubApp(cfg config.AuthHandlerConfig) (*githubApp, error) {
	appID := configStr(cfg.Config, "appId")
	instID := configStr(cfg.Config, "installationId")
	keyPath := configStr(cfg.Config, "privateKeyPath")
	if appID == "" || instID == "" || keyPath == "" {
		return nil, fmt.Errorf("github-app: appId, installationId, and privateKeyPath are required")
	}
	return &githubApp{
		name:           cfg.Name,
		appID:          appID,
		installationID: instID,
		privateKeyPath: keyPath,
	}, nil
}

func (g *githubApp) Name() string { return g.name }

func (g *githubApp) GetToken(_ context.Context) (*Token, error) {
	// TODO: implement JWT signing and installation token exchange
	return nil, fmt.Errorf("github-app token generation not yet implemented")
}

// oauth2ClientCredentials generates tokens via OAuth2 client credentials flow.
type oauth2ClientCredentials struct {
	name         string
	tokenURL     string
	clientID     string
	clientSecret string
	scopes       []string
}

func newOAuth2ClientCredentials(cfg config.AuthHandlerConfig) (*oauth2ClientCredentials, error) {
	tokenURL := configStr(cfg.Config, "tokenUrl")
	clientID := configStr(cfg.Config, "clientId")
	clientSecret := configStr(cfg.Config, "clientSecret")
	if tokenURL == "" || clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("oauth2-client-credentials: tokenUrl, clientId, and clientSecret are required")
	}
	return &oauth2ClientCredentials{
		name:         cfg.Name,
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		scopes:       cfg.DefaultScopes,
	}, nil
}

func (o *oauth2ClientCredentials) Name() string { return o.name }

func (o *oauth2ClientCredentials) GetToken(_ context.Context) (*Token, error) {
	// TODO: implement OAuth2 client credentials token exchange with caching
	return nil, fmt.Errorf("oauth2-client-credentials token generation not yet implemented")
}

// serviceAccount uses a GCP service account key for generating access tokens.
type serviceAccount struct {
	name    string
	keyFile string
	scopes  []string
}

func newServiceAccount(cfg config.AuthHandlerConfig) (*serviceAccount, error) {
	keyFile := configStr(cfg.Config, "keyFile")
	if keyFile == "" {
		return nil, fmt.Errorf("service-account: keyFile is required")
	}
	return &serviceAccount{
		name:    cfg.Name,
		keyFile: keyFile,
		scopes:  cfg.DefaultScopes,
	}, nil
}

func (s *serviceAccount) Name() string { return s.name }

func (s *serviceAccount) GetToken(_ context.Context) (*Token, error) {
	// TODO: implement GCP service account token generation
	return nil, fmt.Errorf("service-account token generation not yet implemented")
}
