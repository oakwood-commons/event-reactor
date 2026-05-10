package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oakwood-commons/event-reactor/pkg/config"
)

func TestStaticToken(t *testing.T) {
	reg, err := NewRegistry([]config.AuthHandlerConfig{
		{
			Name: "my-token",
			Type: "static-token",
			Config: map[string]any{
				"token": "abc123",
			},
		},
	})
	require.NoError(t, err)

	tok, err := reg.GetToken(context.Background(), "my-token")
	require.NoError(t, err)
	assert.Equal(t, "abc123", tok.Value)
	assert.Equal(t, "Bearer", tok.Type)
	assert.Equal(t, "Bearer abc123", tok.Header())
}

func TestGitHubToken(t *testing.T) {
	reg, err := NewRegistry([]config.AuthHandlerConfig{
		{
			Name: "gh",
			Type: "github-token",
			Config: map[string]any{
				"token": "ghp_xxx",
			},
		},
	})
	require.NoError(t, err)

	tok, err := reg.GetToken(context.Background(), "gh")
	require.NoError(t, err)
	assert.Equal(t, "ghp_xxx", tok.Value)
	assert.Equal(t, "token ghp_xxx", tok.Header())
}

func TestRegistry_NotFound(t *testing.T) {
	reg, err := NewRegistry(nil)
	require.NoError(t, err)

	_, err = reg.GetToken(context.Background(), "missing")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRegistry_Has(t *testing.T) {
	reg, err := NewRegistry([]config.AuthHandlerConfig{
		{
			Name:   "test",
			Type:   "static-token",
			Config: map[string]any{"token": "x"},
		},
	})
	require.NoError(t, err)

	assert.True(t, reg.Has("test"))
	assert.False(t, reg.Has("nope"))
}

func TestUnsupportedType(t *testing.T) {
	_, err := NewRegistry([]config.AuthHandlerConfig{
		{Name: "bad", Type: "unknown", Config: map[string]any{}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestGitHubApp_Validation(t *testing.T) {
	_, err := NewRegistry([]config.AuthHandlerConfig{
		{
			Name:   "app",
			Type:   "github-app",
			Config: map[string]any{"appId": "123"},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "installationId")
}

func TestOAuth2_Validation(t *testing.T) {
	_, err := NewRegistry([]config.AuthHandlerConfig{
		{
			Name:   "oauth",
			Type:   "oauth2-client-credentials",
			Config: map[string]any{"tokenUrl": "https://auth.example.com/token"},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "clientId")
}

func TestStaticToken_CustomType(t *testing.T) {
	reg, err := NewRegistry([]config.AuthHandlerConfig{
		{
			Name:   "custom",
			Type:   "static-token",
			Config: map[string]any{"token": "xyz", "tokenType": "Basic"},
		},
	})
	require.NoError(t, err)

	tok, err := reg.GetToken(context.Background(), "custom")
	require.NoError(t, err)
	assert.Equal(t, "Basic xyz", tok.Header())
}

func TestToken_Header_EmptyType(t *testing.T) {
	tok := Token{Value: "abc123", Type: ""}
	assert.Equal(t, "Bearer abc123", tok.Header())
}

func TestStaticToken_MissingToken(t *testing.T) {
	_, err := NewRegistry([]config.AuthHandlerConfig{
		{
			Name:   "empty",
			Type:   "static-token",
			Config: map[string]any{},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token is required")
}

func TestGitHubToken_MissingToken(t *testing.T) {
	_, err := NewRegistry([]config.AuthHandlerConfig{
		{
			Name:   "gh-empty",
			Type:   "github-token",
			Config: map[string]any{},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token is required")
}

func TestServiceAccount_Validation(t *testing.T) {
	_, err := NewRegistry([]config.AuthHandlerConfig{
		{
			Name:   "sa",
			Type:   "service-account",
			Config: map[string]any{},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "keyFile")
}

func TestServiceAccount_Success(t *testing.T) {
	reg, err := NewRegistry([]config.AuthHandlerConfig{
		{
			Name:          "sa",
			Type:          "service-account",
			DefaultScopes: []string{"https://www.googleapis.com/auth/cloud-platform"},
			Config:        map[string]any{"keyFile": "/etc/sa-key.json"},
		},
	})
	require.NoError(t, err)
	assert.True(t, reg.Has("sa"))

	// GetToken returns not-implemented error for placeholder
	_, err = reg.GetToken(context.Background(), "sa")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}
