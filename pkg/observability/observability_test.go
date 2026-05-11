package observability

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oakwood-commons/event-reactor/pkg/config"
)

func TestLogger_JSONDefault(t *testing.T) {
	logger := Logger(config.LoggingConfig{Level: "info", Format: "json"})
	require.NotNil(t, logger)
	// Verify it can log without panicking
	logger.Info("test message")
}

func TestLogger_TextFormat(t *testing.T) {
	logger := Logger(config.LoggingConfig{Level: "debug", Format: "text"})
	require.NotNil(t, logger)
	logger.Debug("test debug")
}

func TestLogger_Levels(t *testing.T) {
	tests := []struct {
		level string
	}{
		{"debug"},
		{"info"},
		{"warn"},
		{"error"},
		{"unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.level, func(t *testing.T) {
			logger := Logger(config.LoggingConfig{Level: tc.level})
			require.NotNil(t, logger)
		})
	}
}

func TestShutdown_Run(t *testing.T) {
	var called int
	s := &Shutdown{}
	s.Add(func(_ context.Context) error {
		called++
		return nil
	})
	s.Add(func(_ context.Context) error {
		called++
		return nil
	})

	err := s.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, called)
}

func TestShutdown_RunWithErrors(t *testing.T) {
	s := &Shutdown{}
	s.Add(func(_ context.Context) error {
		return errors.New("fail-1")
	})
	s.Add(func(_ context.Context) error {
		return nil
	})
	s.Add(func(_ context.Context) error {
		return errors.New("fail-2")
	})

	err := s.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fail-1")
	assert.Contains(t, err.Error(), "fail-2")
}

func TestShutdown_Empty(t *testing.T) {
	s := &Shutdown{}
	err := s.Run(context.Background())
	require.NoError(t, err)
}
