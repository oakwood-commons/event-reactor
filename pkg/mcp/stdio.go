package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
)

// JSONRPCRequest is a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

// Error is a JSON-RPC 2.0 error.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// InitializeResult is returned by initialize.
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
	Capabilities    Capabilities `json:"capabilities"`
}

// ServerInfo describes the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Capabilities describes server capabilities.
type Capabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// ToolsCapability indicates the server can serve tools.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ListToolsResult wraps tool listing.
type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

// CallToolParams are params for tools/call.
type CallToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

const (
	mcpProtocolVersion = "2024-11-05"
	serverName         = "event-reactor"
)

// ServeStdio runs the MCP server over stdin/stdout using JSON-RPC 2.0.
func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer, serverVersion string) error {
	scanner := bufio.NewScanner(in)
	// Allow up to 1MB messages
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.logger.Warn("invalid JSON-RPC request", slog.String("error", err.Error()))
			continue
		}

		resp := s.handleRequest(ctx, req, serverVersion)
		if resp == nil {
			// Notification -- no response needed
			continue
		}

		b, err := json.Marshal(resp)
		if err != nil {
			s.logger.Error("marshaling response", slog.String("error", err.Error()))
			continue
		}

		if _, err := fmt.Fprintf(out, "%s\n", b); err != nil {
			return fmt.Errorf("writing response: %w", err)
		}
	}

	return scanner.Err()
}

func (s *Server) handleRequest(ctx context.Context, req JSONRPCRequest, serverVersion string) *JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: InitializeResult{
				ProtocolVersion: mcpProtocolVersion,
				ServerInfo: ServerInfo{
					Name:    serverName,
					Version: serverVersion,
				},
				Capabilities: Capabilities{
					Tools: &ToolsCapability{},
				},
			},
		}

	case "notifications/initialized":
		return nil // notification, no response

	case "tools/list":
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  ListToolsResult{Tools: s.ListTools()},
		}

	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &Error{Code: -32602, Message: fmt.Sprintf("invalid params: %v", err)},
			}
		}
		result := s.CallTool(ctx, params.Name, params.Arguments)
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  result,
		}

	case "ping":
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{},
		}

	default:
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
		}
	}
}
