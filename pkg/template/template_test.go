package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testData() map[string]any {
	return map[string]any{
		"payload": map[string]any{
			"action": "opened",
			"number": 42,
			"repository": map[string]any{
				"name": "event-reactor",
				"owner": map[string]any{
					"login": "oakwood-commons",
				},
			},
		},
		"attributes": map[string]string{
			"eventType": "pull_request",
			"env":       "prod",
		},
		"id":     "msg-001",
		"source": "pubsub",
		"type":   "pull_request",
	}
}

func TestRender_SimpleAccess(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		want string
	}{
		{
			name: "payload field",
			tmpl: "{{ .payload.action }}",
			want: "opened",
		},
		{
			name: "nested payload",
			tmpl: "{{ .payload.repository.owner.login }}/{{ .payload.repository.name }}",
			want: "oakwood-commons/event-reactor",
		},
		{
			name: "attributes",
			tmpl: "env={{ index .attributes \"env\" }}",
			want: "env=prod",
		},
		{
			name: "id and source",
			tmpl: "{{ .id }} from {{ .source }}",
			want: "msg-001 from pubsub",
		},
		{
			name: "missing key returns zero",
			tmpl: "val={{ .payload.missing }}",
			want: "val=<no value>",
		},
	}

	data := testData()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Render(tc.tmpl, data)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRender_CustomFunctions(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		want string
	}{
		{
			name: "upper",
			tmpl: `{{ upper "hello" }}`,
			want: "HELLO",
		},
		{
			name: "lower",
			tmpl: `{{ lower "WORLD" }}`,
			want: "world",
		},
		{
			name: "trimSpace",
			tmpl: `{{ trimSpace "  hi  " }}`,
			want: "hi",
		},
		{
			name: "contains",
			tmpl: `{{ if contains "hello world" "world" }}yes{{ end }}`,
			want: "yes",
		},
		{
			name: "hasPrefix",
			tmpl: `{{ if hasPrefix "event-reactor" "event" }}yes{{ end }}`,
			want: "yes",
		},
		{
			name: "replace",
			tmpl: `{{ replace "foo-bar-baz" "-" "_" }}`,
			want: "foo_bar_baz",
		},
		{
			name: "join",
			tmpl: `{{ $s := split "a,b,c" "," }}{{ join $s "-" }}`,
			want: "a-b-c",
		},
		{
			name: "default with value",
			tmpl: `{{ default "fallback" .payload.action }}`,
			want: "opened",
		},
		{
			name: "default with missing",
			tmpl: `{{ default "fallback" .payload.missing }}`,
			want: "fallback",
		},
		{
			name: "toJSON",
			tmpl: `{{ toJSON .payload.action }}`,
			want: `"opened"`,
		},
	}

	data := testData()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Render(tc.tmpl, data)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRender_MultilineTemplate(t *testing.T) {
	tmpl := `## Review Checklist for {{ .payload.repository.name }}
PR #{{ .payload.number }} ({{ .payload.action }})
- [ ] Tests pass
- [ ] Docs updated`

	got, err := Render(tmpl, testData())
	require.NoError(t, err)
	assert.Contains(t, got, "Review Checklist for event-reactor")
	assert.Contains(t, got, "PR #42 (opened)")
	assert.Contains(t, got, "- [ ] Tests pass")
}

func TestRender_InvalidTemplate(t *testing.T) {
	_, err := Render("{{ invalid }", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing template")
}
