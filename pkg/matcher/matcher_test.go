package matcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oakwood-commons/event-reactor/pkg/message"
)

func newTestEvent() message.Event {
	return message.Event{
		ID:     "msg-001",
		Source: "pubsub",
		Type:   "pull_request",
		Attributes: map[string]string{
			"eventType": "pull_request",
			"env":       "prod",
		},
		Payload: map[string]any{
			"action": "opened",
			"number": 42,
			"pull_request": map[string]any{
				"base": map[string]any{
					"ref": "main",
				},
			},
		},
	}
}

func TestMatcher_Match(t *testing.T) {
	m, err := New()
	require.NoError(t, err)

	ev := newTestEvent()

	tests := []struct {
		name    string
		expr    string
		want    bool
		wantErr bool
	}{
		{
			name: "empty expression matches all",
			expr: "",
			want: true,
		},
		{
			name: "literal true",
			expr: "true",
			want: true,
		},
		{
			name: "literal false",
			expr: "false",
			want: false,
		},
		{
			name: "match payload field",
			expr: `payload.action == "opened"`,
			want: true,
		},
		{
			name: "no match payload field",
			expr: `payload.action == "closed"`,
			want: false,
		},
		{
			name: "match attributes",
			expr: `attributes.eventType == "pull_request"`,
			want: true,
		},
		{
			name: "match id",
			expr: `id == "msg-001"`,
			want: true,
		},
		{
			name: "match source",
			expr: `source == "pubsub"`,
			want: true,
		},
		{
			name: "match type",
			expr: `type == "pull_request"`,
			want: true,
		},
		{
			name: "nested payload access",
			expr: `payload.pull_request.base.ref == "main"`,
			want: true,
		},
		{
			name: "compound expression",
			expr: `payload.action == "opened" && attributes.env == "prod"`,
			want: true,
		},
		{
			name: "has() macro",
			expr: `has(payload.action)`,
			want: true,
		},
		{
			name: "has() macro missing field",
			expr: `has(payload.missing_field)`,
			want: false,
		},
		{
			name: "in operator",
			expr: `attributes.env in ["prod", "staging"]`,
			want: true,
		},
		{
			name:    "invalid expression",
			expr:    `invalid %%% expression`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := m.Match(tc.expr, ev)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMatcher_Compile_Caching(t *testing.T) {
	m, err := New()
	require.NoError(t, err)

	expr := `payload.action == "opened"`

	prg1, err := m.Compile(expr)
	require.NoError(t, err)

	prg2, err := m.Compile(expr)
	require.NoError(t, err)

	// Same program instance from cache
	assert.Equal(t, prg1, prg2)
}

func TestMatcher_Compile_InvalidExpression(t *testing.T) {
	m, err := New()
	require.NoError(t, err)

	_, err = m.Compile("bad %%% expr")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compiling CEL expression")
}

func TestNew_Success(t *testing.T) {
	m, err := New()
	require.NoError(t, err)
	assert.NotNil(t, m)
}
