// Package listener defines the Listener interface and common types
// for event source implementations.
package listener

import (
	"context"

	"github.com/oakwood-commons/event-reactor/pkg/message"
)

// Listener subscribes to an event source and delivers normalized events.
type Listener interface {
	// Start begins listening for events. It blocks until the context
	// is cancelled or a fatal error occurs. Events are delivered via
	// the handler callback.
	Start(ctx context.Context, handler func(context.Context, message.Event)) error

	// Name returns the configured name of this listener instance.
	Name() string
}
