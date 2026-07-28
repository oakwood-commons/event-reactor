// Package pubsub implements the GCP Pub/Sub pull subscription listener.
// It consumes events from a subscription via streaming pull and normalizes
// each message into a message.Event before handing it to the pipeline.
package pubsub

import (
	"context"
	"fmt"
	"log/slog"

	"cloud.google.com/go/pubsub/v2"
	"google.golang.org/api/option"

	"github.com/oakwood-commons/event-reactor/pkg/message"
)

// Config configures the Pub/Sub pull listener.
//
// AckDeadline is intentionally not configured here: it is a property of the
// subscription itself (set when the subscription is provisioned) rather than a
// client-side receive setting.
type Config struct {
	// Name is the listener instance name (used in logs and errors).
	Name string
	// ProjectID is the GCP project that owns the subscription.
	ProjectID string
	// SubscriptionID is the Pub/Sub subscription to pull from.
	SubscriptionID string
	// MaxOutstandingMessages bounds the number of unacknowledged messages the
	// client will hold at once. Zero uses the client default.
	MaxOutstandingMessages int
	// NumGoroutines is the number of streaming-pull goroutines the client runs.
	// Zero uses the client default.
	NumGoroutines int
}

// Listener consumes events from a GCP Pub/Sub subscription via streaming pull.
type Listener struct {
	name                   string
	projectID              string
	subscriptionID         string
	maxOutstandingMessages int
	numGoroutines          int
	logger                 *slog.Logger

	// clientOpts are extra client options. It is used by tests to point the
	// client at an in-process fake (pstest); it is empty in production, where
	// the client uses Application Default Credentials.
	clientOpts []option.ClientOption
}

// New creates a Pub/Sub pull listener. It returns an error if required
// configuration is missing.
func New(cfg Config, logger *slog.Logger) (*Listener, error) {
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("pubsub listener %q: projectId is required", cfg.Name)
	}
	if cfg.SubscriptionID == "" {
		return nil, fmt.Errorf("pubsub listener %q: subscriptionId is required", cfg.Name)
	}
	return &Listener{
		name:                   cfg.Name,
		projectID:              cfg.ProjectID,
		subscriptionID:         cfg.SubscriptionID,
		maxOutstandingMessages: cfg.MaxOutstandingMessages,
		numGoroutines:          cfg.NumGoroutines,
		logger:                 logger,
	}, nil
}

// Name returns the configured name of this listener instance.
func (l *Listener) Name() string { return l.name }

// Start opens a streaming pull against the subscription and delivers each
// normalized event to handler. It blocks until ctx is cancelled or an
// unrecoverable error occurs.
//
// Acknowledgement semantics: a message is acked once handler returns. Messages
// that cannot be converted into an event, or that cause handler to panic, are
// nacked so Pub/Sub redelivers them (and eventually routes them to the
// dead-letter topic once the subscription's max delivery attempts are reached).
// If the process exits before a message is acked, Pub/Sub redelivers it.
func (l *Listener) Start(ctx context.Context, handler func(context.Context, message.Event)) error {
	client, err := pubsub.NewClient(ctx, l.projectID, l.clientOpts...)
	if err != nil {
		return fmt.Errorf("pubsub listener %q: creating client: %w", l.name, err)
	}
	defer func() { _ = client.Close() }()

	sub := client.Subscriber(l.subscriptionID)
	if l.maxOutstandingMessages > 0 {
		sub.ReceiveSettings.MaxOutstandingMessages = l.maxOutstandingMessages
	}
	if l.numGoroutines > 0 {
		sub.ReceiveSettings.NumGoroutines = l.numGoroutines
	}

	l.logger.Info("pubsub listener started",
		slog.String("listener", l.name),
		slog.String("project", l.projectID),
		slog.String("subscription", l.subscriptionID),
	)

	err = sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		l.dispatch(ctx, handler, msg)
	})
	// Receive returns nil on a clean shutdown via context cancellation.
	if err != nil && ctx.Err() == nil {
		return fmt.Errorf("pubsub listener %q: receiving: %w", l.name, err)
	}

	l.logger.Info("pubsub listener stopped", slog.String("listener", l.name))
	return nil
}

// dispatch converts a single message and invokes handler, translating the
// outcome into an ack or nack. It recovers from handler panics so that one bad
// message cannot tear down the streaming pull.
func (l *Listener) dispatch(ctx context.Context, handler func(context.Context, message.Event), msg *pubsub.Message) {
	ev, err := message.FromPubSubMessage(msg.ID, msg.Attributes, msg.Data)
	if err != nil {
		l.logger.ErrorContext(ctx, "discarding unconvertible pubsub message; nacking",
			slog.String("listener", l.name),
			slog.String("messageId", msg.ID),
			slog.String("error", err.Error()),
		)
		msg.Nack()
		return
	}

	acked := false
	defer func() {
		if r := recover(); r != nil {
			l.logger.ErrorContext(ctx, "handler panicked; nacking pubsub message",
				slog.String("listener", l.name),
				slog.String("messageId", msg.ID),
				slog.Any("panic", r),
			)
			if !acked {
				msg.Nack()
			}
		}
	}()

	handler(ctx, ev)
	acked = true
	msg.Ack()
}
