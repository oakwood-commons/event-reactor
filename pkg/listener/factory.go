package listener

import (
	"fmt"
	"log/slog"

	"github.com/oakwood-commons/event-reactor/pkg/config"
	"github.com/oakwood-commons/event-reactor/pkg/listener/pubsub"
)

// Build constructs a background Listener from a listener configuration.
//
// Some listener types are served directly by the HTTP API server (for example
// "webhook", "cloudevents", and generic push on "/events") and do not require a
// standalone background listener. For those, Build returns (nil, nil) and the
// caller should skip them. Unknown types return an error so misconfiguration is
// surfaced at startup rather than silently ignored.
func Build(cfg config.ListenerConfig, logger *slog.Logger) (Listener, error) {
	switch cfg.Type {
	case "pubsub":
		return buildPubSub(cfg, logger)
	case "webhook", "cloudevents", "http":
		// Served by the HTTP API server; no background listener required.
		return nil, nil
	default:
		return nil, fmt.Errorf("listener %q: unsupported type %q", cfg.Name, cfg.Type)
	}
}

func buildPubSub(cfg config.ListenerConfig, logger *slog.Logger) (Listener, error) {
	return pubsub.New(pubsub.Config{
		Name:                   cfg.Name,
		ProjectID:              stringField(cfg, "projectId"),
		SubscriptionID:         stringField(cfg, "subscriptionId"),
		MaxOutstandingMessages: intField(cfg, "maxOutstandingMessages"),
		NumGoroutines:          intField(cfg, "numGoroutines"),
	}, logger)
}

// stringField returns the named config value if it is a string, else "".
func stringField(cfg config.ListenerConfig, key string) string {
	if v, ok := cfg.Config[key].(string); ok {
		return v
	}
	return ""
}

// intField returns the named config value as an int, accepting the integer and
// float types that YAML decoding may produce. It returns 0 when unset or of an
// unexpected type.
func intField(cfg config.ListenerConfig, key string) int {
	switch v := cfg.Config[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
