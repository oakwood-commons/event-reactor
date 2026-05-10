// Package mcp implements the MCP (Model Context Protocol) server
// for AI-assisted configuration of event-reactor.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/oakwood-commons/event-reactor/pkg/config"
	"github.com/oakwood-commons/event-reactor/pkg/matcher"
	"github.com/oakwood-commons/event-reactor/pkg/message"
	"github.com/oakwood-commons/event-reactor/pkg/reactor"
	ertmpl "github.com/oakwood-commons/event-reactor/pkg/template"
)

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ToolResult is the response from executing a tool.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is a piece of content in a tool result.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Server is the MCP server for event-reactor.
type Server struct {
	registry *reactor.Registry
	logger   *slog.Logger
	tools    map[string]toolHandler
}

type toolHandler func(ctx context.Context, args map[string]any) ToolResult

// New creates an MCP server with the given provider registry.
func New(reg *reactor.Registry, logger *slog.Logger) *Server {
	s := &Server{
		registry: reg,
		logger:   logger,
	}
	s.tools = map[string]toolHandler{
		"list_providers":      s.listProviders,
		"validate_config":     s.validateConfig,
		"test_cel_expression": s.testCELExpression,
		"render_template":     s.renderTemplate,
		"test_reactor":        s.testReactor,
		"list_event_sources":  s.listEventSources,
	}
	return s
}

// ListTools returns all available MCP tools.
func (s *Server) ListTools() []Tool {
	return []Tool{
		{
			Name:        "list_providers",
			Description: "List all registered reactor providers",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "validate_config",
			Description: "Validate an event-reactor server configuration",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"config": map[string]any{"type": "string", "description": "YAML config content"},
				},
				"required": []string{"config"},
			},
		},
		{
			Name:        "test_cel_expression",
			Description: "Evaluate a CEL expression against an event",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"expression": map[string]any{"type": "string", "description": "CEL expression"},
					"event":      map[string]any{"type": "object", "description": "Event data as JSON"},
				},
				"required": []string{"expression", "event"},
			},
		},
		{
			Name:        "render_template",
			Description: "Render a Go template against event data",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"template": map[string]any{"type": "string", "description": "Go template string"},
					"event":    map[string]any{"type": "object", "description": "Event data as JSON"},
				},
				"required": []string{"template", "event"},
			},
		},
		{
			Name:        "test_reactor",
			Description: "Dry-run a reactor config against an event, resolving all inputs",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"config":  map[string]any{"type": "string", "description": "YAML config content"},
					"reactor": map[string]any{"type": "string", "description": "Reactor name"},
					"event":   map[string]any{"type": "object", "description": "Event data as JSON"},
				},
				"required": []string{"config", "reactor", "event"},
			},
		},
		{
			Name:        "list_event_sources",
			Description: "List supported event source types",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
}

// CallTool executes an MCP tool by name.
func (s *Server) CallTool(ctx context.Context, name string, args map[string]any) ToolResult {
	handler, ok := s.tools[name]
	if !ok {
		return errorResult(fmt.Sprintf("unknown tool: %s", name))
	}
	return handler(ctx, args)
}

func (s *Server) listProviders(_ context.Context, _ map[string]any) ToolResult {
	providers := s.registry.Providers()
	if len(providers) == 0 {
		return textResult("No providers registered")
	}
	b, _ := json.MarshalIndent(providers, "", "  ")
	return textResult(string(b))
}

func (s *Server) validateConfig(_ context.Context, args map[string]any) ToolResult {
	cfgStr, ok := args["config"].(string)
	if !ok {
		return errorResult("config must be a string")
	}

	cfg, err := config.Parse([]byte(cfgStr))
	if err != nil {
		return errorResult(fmt.Sprintf("invalid config: %v", err))
	}

	// Also compile CEL expressions
	m, err := matcher.New()
	if err != nil {
		return errorResult(fmt.Sprintf("creating matcher: %v", err))
	}

	for _, rc := range cfg.Reactors {
		if rc.Match != "" {
			if _, err := m.Compile(rc.Match); err != nil {
				return errorResult(fmt.Sprintf("reactor %q: invalid CEL expression: %v", rc.Name, err))
			}
		}
	}

	return textResult(fmt.Sprintf("Config valid: %d listener(s), %d reactor(s), all CEL expressions valid",
		len(cfg.Listeners), len(cfg.Reactors)))
}

func (s *Server) testCELExpression(_ context.Context, args map[string]any) ToolResult {
	expr, ok := args["expression"].(string)
	if !ok {
		return errorResult("expression must be a string")
	}

	eventData, ok := args["event"].(map[string]any)
	if !ok {
		return errorResult("event must be a JSON object")
	}

	event, err := message.FromGenericPayload(eventData)
	if err != nil {
		return errorResult(fmt.Sprintf("invalid event: %v", err))
	}

	m, err := matcher.New()
	if err != nil {
		return errorResult(fmt.Sprintf("creating matcher: %v", err))
	}

	matched, err := m.Match(expr, event)
	if err != nil {
		return errorResult(fmt.Sprintf("expression error: %v", err))
	}

	if matched {
		return textResult("MATCH: expression evaluated to true")
	}
	return textResult("NO MATCH: expression evaluated to false")
}

func (s *Server) renderTemplate(_ context.Context, args map[string]any) ToolResult {
	tmpl, ok := args["template"].(string)
	if !ok {
		return errorResult("template must be a string")
	}

	eventData, ok := args["event"].(map[string]any)
	if !ok {
		return errorResult("event must be a JSON object")
	}

	event, err := message.FromGenericPayload(eventData)
	if err != nil {
		return errorResult(fmt.Sprintf("invalid event: %v", err))
	}

	result, err := ertmpl.Render(tmpl, event.AsMap())
	if err != nil {
		return errorResult(fmt.Sprintf("template error: %v", err))
	}

	return textResult(result)
}

func (s *Server) testReactor(_ context.Context, args map[string]any) ToolResult {
	cfgStr, ok := args["config"].(string)
	if !ok {
		return errorResult("config must be a string")
	}

	reactorName, ok := args["reactor"].(string)
	if !ok {
		return errorResult("reactor must be a string")
	}

	eventData, ok := args["event"].(map[string]any)
	if !ok {
		return errorResult("event must be a JSON object")
	}

	cfg, err := config.Parse([]byte(cfgStr))
	if err != nil {
		return errorResult(fmt.Sprintf("invalid config: %v", err))
	}

	event, err := message.FromGenericPayload(eventData)
	if err != nil {
		return errorResult(fmt.Sprintf("invalid event: %v", err))
	}

	m, err := matcher.New()
	if err != nil {
		return errorResult(fmt.Sprintf("creating matcher: %v", err))
	}

	var rc *config.ReactorConfig
	for i := range cfg.Reactors {
		if cfg.Reactors[i].Name == reactorName {
			rc = &cfg.Reactors[i]
			break
		}
	}
	if rc == nil {
		return errorResult(fmt.Sprintf("reactor %q not found", reactorName))
	}

	matched, err := m.Match(rc.Match, event)
	if err != nil {
		return errorResult(fmt.Sprintf("match error: %v", err))
	}
	if !matched {
		return textResult("NO MATCH: event does not match reactor expression")
	}

	resolved, err := reactor.ResolveInputs(*rc, event, m)
	if err != nil {
		return errorResult(fmt.Sprintf("input resolution error: %v", err))
	}

	b, _ := json.MarshalIndent(map[string]any{
		"matched":  true,
		"reactor":  rc.Name,
		"provider": rc.Provider,
		"inputs":   resolved,
	}, "", "  ")

	return textResult(string(b))
}

func (s *Server) listEventSources(_ context.Context, _ map[string]any) ToolResult {
	sources := []map[string]string{
		{"type": "pubsub", "description": "GCP Pub/Sub pull subscription"},
		{"type": "cloudevents", "description": "CloudEvents HTTP endpoint (CNCF spec)"},
		{"type": "webhook", "description": "Generic webhook with HMAC validation"},
	}
	b, _ := json.MarshalIndent(sources, "", "  ")
	return textResult(string(b))
}

func textResult(text string) ToolResult {
	return ToolResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
	}
}

func errorResult(text string) ToolResult {
	return ToolResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
		IsError: true,
	}
}
