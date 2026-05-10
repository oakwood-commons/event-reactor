// Package template provides Go template rendering for event-reactor.
// Templates have access to event data (payload, attributes, id, source, type).
package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// Render executes a Go template string against the given data map.
// The data map typically comes from message.Event.AsMap().
func Render(tmpl string, data map[string]any) (string, error) {
	t, err := template.New("").
		Option("missingkey=zero").
		Funcs(funcMap()).
		Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}

// funcMap returns the custom template functions available in event-reactor templates.
func funcMap() template.FuncMap {
	return template.FuncMap{
		"upper":      strings.ToUpper,
		"lower":      strings.ToLower,
		"trimSpace":  strings.TrimSpace,
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		"replace":    strings.ReplaceAll,
		"split":      strings.Split,
		"join":       strings.Join,
		"trimPrefix": strings.TrimPrefix,
		"trimSuffix": strings.TrimSuffix,
		"default":    defaultVal,
		"toJSON":     toJSON,
	}
}

// defaultVal returns val if non-empty, otherwise dflt.
func defaultVal(dflt, val any) any {
	if val == nil {
		return dflt
	}
	if s, ok := val.(string); ok && s == "" {
		return dflt
	}
	return val
}

// toJSON renders a value as a JSON string.
func toJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
