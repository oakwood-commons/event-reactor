package listener

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oakwood-commons/event-reactor/pkg/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestBuild(t *testing.T) {
	tests := []struct {
		name     string
		cfg      config.ListenerConfig
		wantNil  bool
		wantErr  string
		wantName string
	}{
		{
			name: "pubsub with required fields",
			cfg: config.ListenerConfig{
				Name: "events",
				Type: "pubsub",
				Config: map[string]any{
					"projectId":              "my-project",
					"subscriptionId":         "my-sub",
					"maxOutstandingMessages": 100,
					"numGoroutines":          2,
				},
			},
			wantName: "events",
		},
		{
			name: "pubsub missing projectId",
			cfg: config.ListenerConfig{
				Name:   "events",
				Type:   "pubsub",
				Config: map[string]any{"subscriptionId": "my-sub"},
			},
			wantErr: "projectId is required",
		},
		{
			name: "pubsub missing subscriptionId",
			cfg: config.ListenerConfig{
				Name:   "events",
				Type:   "pubsub",
				Config: map[string]any{"projectId": "my-project"},
			},
			wantErr: "subscriptionId is required",
		},
		{
			name:    "webhook is served by the HTTP server",
			cfg:     config.ListenerConfig{Name: "hooks", Type: "webhook"},
			wantNil: true,
		},
		{
			name:    "cloudevents is served by the HTTP server",
			cfg:     config.ListenerConfig{Name: "ce", Type: "cloudevents"},
			wantNil: true,
		},
		{
			name:    "unsupported type errors",
			cfg:     config.ListenerConfig{Name: "mystery", Type: "kafka"},
			wantErr: `unsupported type "kafka"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l, err := Build(tc.cfg, testLogger())
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.wantNil {
				assert.Nil(t, l)
				return
			}
			require.NotNil(t, l)
			assert.Equal(t, tc.wantName, l.Name())
		})
	}
}

func TestIntField(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want int
	}{
		{name: "int", val: 42, want: 42},
		{name: "int64", val: int64(7), want: 7},
		{name: "float64", val: float64(9), want: 9},
		{name: "string is ignored", val: "10", want: 0},
		{name: "missing", val: nil, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.ListenerConfig{Config: map[string]any{}}
			if tc.val != nil {
				cfg.Config["n"] = tc.val
			}
			assert.Equal(t, tc.want, intField(cfg, "n"))
		})
	}
}
