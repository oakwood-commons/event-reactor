// Package message defines the event envelope types used throughout event-reactor.
// All listeners normalize incoming events into this common format before
// passing them to the matcher and reactor pipeline.
package message

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// Event is the normalized event envelope produced by listeners.
// It carries both attributes (metadata) and payload (body) in a
// format that CEL expressions and Go templates can evaluate.
//
// CEL variables: payload.*, attributes.*, id, source, type
type Event struct {
	// ID is a unique identifier for the event, typically from the source system.
	ID string `json:"id" yaml:"id"`

	// Source identifies where the event originated (e.g., "pubsub/my-subscription").
	Source string `json:"source" yaml:"source"`

	// Type classifies the event (e.g., "google.cloud.pubsub.topic.v1.messagePublished").
	Type string `json:"type" yaml:"type"`

	// Time is when the event was produced.
	Time time.Time `json:"time" yaml:"time"`

	// Attributes are string key-value metadata about the event
	// (e.g., eventType, source, timestamp). Listeners extract these
	// from the underlying transport (Pub/Sub attributes, HTTP headers, etc.).
	Attributes map[string]string `json:"attributes" yaml:"attributes"`

	// Payload is the decoded event body. It is typically a
	// map[string]any from JSON, but may be any type the listener produces.
	Payload any `json:"payload" yaml:"payload"`
}

// AsMap returns the event as a map suitable for CEL expression evaluation.
// The returned map has keys: "payload", "attributes", "id", "source", "type".
func (e Event) AsMap() map[string]any {
	return map[string]any{
		"payload":    e.Payload,
		"attributes": e.Attributes,
		"id":         e.ID,
		"source":     e.Source,
		"type":       e.Type,
	}
}

// FromPubSubPush creates an Event from a GCP Pub/Sub push message envelope.
// The envelope is expected to have the structure:
//
//	{"message": {"data": "<base64>", "attributes": {...}, "messageId": "..."}}
func FromPubSubPush(envelope map[string]any) (Event, error) {
	msg, ok := envelope["message"]
	if !ok {
		return Event{}, fmt.Errorf("missing 'message' field in Pub/Sub envelope")
	}

	msgMap, ok := msg.(map[string]any)
	if !ok {
		return Event{}, fmt.Errorf("'message' field is not an object")
	}

	ev := Event{
		Attributes: extractAttributes(msgMap),
		Time:       time.Now(),
		Source:     "pubsub",
	}

	ev.ID, _ = msgMap["messageId"].(string)

	payload, err := decodeData(msgMap["data"])
	if err != nil {
		return Event{}, fmt.Errorf("decoding Pub/Sub data: %w", err)
	}
	ev.Payload = payload

	return ev, nil
}

// FromGenericPayload creates an Event from an already-decoded payload.
// It supports map[string]any, []map[string]any, and []any shapes.
func FromGenericPayload(payload any) (Event, error) {
	ev := Event{
		Time: time.Now(),
	}

	switch p := payload.(type) {
	case map[string]any:
		ev.Attributes = extractAttributes(p)
		ev.Payload = p
	case []map[string]any:
		ev.Payload = map[string]any{"items": p}
	case []any:
		ev.Payload = map[string]any{"items": p}
	default:
		return Event{}, fmt.Errorf("unsupported payload type: %T", payload)
	}

	return ev, nil
}

// decodeData decodes the "data" field from a Pub/Sub message.
// It handles string (raw JSON or base64-encoded JSON) and []byte.
func decodeData(raw any) (any, error) {
	switch d := raw.(type) {
	case string:
		return decodeStringData(d)
	case []byte:
		var result map[string]any
		if err := json.Unmarshal(d, &result); err != nil {
			return nil, fmt.Errorf("unmarshaling data bytes: %w", err)
		}
		return result, nil
	case nil:
		return map[string]any{}, nil
	default:
		return nil, fmt.Errorf("unexpected data type: %T", raw)
	}
}

// decodeStringData tries to parse a string as JSON first, then as base64-encoded JSON.
func decodeStringData(s string) (any, error) {
	var result map[string]any
	if err := json.Unmarshal([]byte(s), &result); err == nil {
		return result, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("data is neither JSON nor valid base64: %w", err)
	}

	if err := json.Unmarshal(decoded, &result); err != nil {
		return nil, fmt.Errorf("base64-decoded data is not valid JSON: %w", err)
	}
	return result, nil
}

// extractAttributes extracts the "attributes" key from a map and converts all values to strings.
func extractAttributes(m map[string]any) map[string]string {
	raw, ok := m["attributes"]
	if !ok {
		return map[string]string{}
	}

	switch a := raw.(type) {
	case map[string]any:
		attrs := make(map[string]string, len(a))
		for k, v := range a {
			attrs[k] = fmt.Sprintf("%v", v)
		}
		return attrs
	case map[string]string:
		return a
	default:
		return map[string]string{}
	}
}
