package message

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvent_AsMap(t *testing.T) {
	ev := Event{
		ID:     "msg-123",
		Source: "pubsub",
		Type:   "test.event",
		Attributes: map[string]string{
			"eventType": "push",
		},
		Payload: map[string]any{
			"action": "opened",
		},
	}

	m := ev.AsMap()
	assert.Equal(t, "msg-123", m["id"])
	assert.Equal(t, "pubsub", m["source"])
	assert.Equal(t, "test.event", m["type"])
	assert.Equal(t, map[string]string{"eventType": "push"}, m["attributes"])
	assert.Equal(t, map[string]any{"action": "opened"}, m["payload"])
}

func TestFromPubSubPush(t *testing.T) {
	tests := []struct {
		name     string
		envelope map[string]any
		wantErr  string
		check    func(t *testing.T, ev Event)
	}{
		{
			name: "valid JSON data",
			envelope: map[string]any{
				"message": map[string]any{
					"data":      `{"action":"opened","number":42}`,
					"messageId": "msg-001",
					"attributes": map[string]any{
						"eventType": "pull_request",
					},
				},
			},
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, "msg-001", ev.ID)
				assert.Equal(t, "pubsub", ev.Source)
				assert.Equal(t, "pull_request", ev.Attributes["eventType"])
				payload, ok := ev.Payload.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "opened", payload["action"])
				assert.Equal(t, float64(42), payload["number"])
			},
		},
		{
			name: "base64 encoded data",
			envelope: map[string]any{
				"message": map[string]any{
					"data":      base64.StdEncoding.EncodeToString([]byte(`{"key":"value"}`)),
					"messageId": "msg-002",
				},
			},
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, "msg-002", ev.ID)
				payload, ok := ev.Payload.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "value", payload["key"])
			},
		},
		{
			name: "byte slice data",
			envelope: map[string]any{
				"message": map[string]any{
					"data":      []byte(`{"status":"ok"}`),
					"messageId": "msg-003",
				},
			},
			check: func(t *testing.T, ev Event) {
				payload, ok := ev.Payload.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "ok", payload["status"])
			},
		},
		{
			name: "nil data",
			envelope: map[string]any{
				"message": map[string]any{
					"messageId": "msg-004",
				},
			},
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, map[string]any{}, ev.Payload)
			},
		},
		{
			name: "string attributes",
			envelope: map[string]any{
				"message": map[string]any{
					"data": `{}`,
					"attributes": map[string]string{
						"source": "github",
					},
				},
			},
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, "github", ev.Attributes["source"])
			},
		},
		{
			name:     "missing message field",
			envelope: map[string]any{},
			wantErr:  "missing 'message' field",
		},
		{
			name: "message is not an object",
			envelope: map[string]any{
				"message": "not-a-map",
			},
			wantErr: "'message' field is not an object",
		},
		{
			name: "invalid data",
			envelope: map[string]any{
				"message": map[string]any{
					"data": "not-json-and-not-base64!!!",
				},
			},
			wantErr: "decoding Pub/Sub data",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := FromPubSubPush(tc.envelope)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.check(t, ev)
		})
	}
}

func TestFromGenericPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload any
		wantErr string
		check   func(t *testing.T, ev Event)
	}{
		{
			name: "map payload",
			payload: map[string]any{
				"action": "created",
				"attributes": map[string]any{
					"env": "prod",
				},
			},
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, "prod", ev.Attributes["env"])
				p, ok := ev.Payload.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "created", p["action"])
			},
		},
		{
			name: "slice of maps payload",
			payload: []map[string]any{
				{"name": "a"},
				{"name": "b"},
			},
			check: func(t *testing.T, ev Event) {
				p, ok := ev.Payload.(map[string]any)
				require.True(t, ok)
				items, ok := p["items"].([]map[string]any)
				require.True(t, ok)
				assert.Len(t, items, 2)
			},
		},
		{
			name:    "slice of any payload",
			payload: []any{"x", "y"},
			check: func(t *testing.T, ev Event) {
				p, ok := ev.Payload.(map[string]any)
				require.True(t, ok)
				items, ok := p["items"].([]any)
				require.True(t, ok)
				assert.Equal(t, []any{"x", "y"}, items)
			},
		},
		{
			name:    "unsupported type",
			payload: 42,
			wantErr: "unsupported payload type",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := FromGenericPayload(tc.payload)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.check(t, ev)
		})
	}
}

func TestEvent_JSONRoundTrip(t *testing.T) {
	ev := Event{
		ID:     "test-id",
		Source: "webhook",
		Type:   "pull_request",
		Attributes: map[string]string{
			"eventType": "push",
		},
		Payload: map[string]any{
			"action": "opened",
		},
	}

	data, err := json.Marshal(ev)
	require.NoError(t, err)

	var decoded Event
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, ev.ID, decoded.ID)
	assert.Equal(t, ev.Source, decoded.Source)
	assert.Equal(t, ev.Type, decoded.Type)
	assert.Equal(t, ev.Attributes, decoded.Attributes)
}

func TestExtractAttributes(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		want map[string]string
	}{
		{
			name: "no attributes key",
			m:    map[string]any{"foo": "bar"},
			want: map[string]string{},
		},
		{
			name: "map[string]any attributes",
			m: map[string]any{
				"attributes": map[string]any{
					"count": 5,
					"name":  "test",
				},
			},
			want: map[string]string{"count": "5", "name": "test"},
		},
		{
			name: "map[string]string attributes",
			m: map[string]any{
				"attributes": map[string]string{
					"a": "b",
				},
			},
			want: map[string]string{"a": "b"},
		},
		{
			name: "unsupported attributes type",
			m: map[string]any{
				"attributes": "not-a-map",
			},
			want: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractAttributes(tc.m)
			assert.Equal(t, tc.want, got)
		})
	}
}
