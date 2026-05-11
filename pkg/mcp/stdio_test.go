package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sendRequest(t *testing.T, method string, id, params any) string {
	t.Helper()
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
	}
	if params != nil {
		b, err := json.Marshal(params)
		require.NoError(t, err)
		req.Params = b
	}
	line, err := json.Marshal(req)
	require.NoError(t, err)
	return string(line)
}

func TestServeStdio_Initialize(t *testing.T) {
	s := testServer(t)
	line := sendRequest(t, "initialize", 1, nil)

	in := strings.NewReader(line + "\n")
	var out bytes.Buffer

	err := s.ServeStdio(context.Background(), in, &out, "1.0.0-test")
	require.NoError(t, err)

	var resp JSONRPCResponse
	require.NoError(t, json.Unmarshal(out.Bytes(), &resp))

	assert.Equal(t, "2.0", resp.JSONRPC)
	assert.Equal(t, float64(1), resp.ID)
	assert.Nil(t, resp.Error)

	result, ok := resp.Result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, mcpProtocolVersion, result["protocolVersion"])

	info, ok := result["serverInfo"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, serverName, info["name"])
	assert.Equal(t, "1.0.0-test", info["version"])
}

func TestServeStdio_ToolsList(t *testing.T) {
	s := testServer(t)
	line := sendRequest(t, "tools/list", 2, nil)

	in := strings.NewReader(line + "\n")
	var out bytes.Buffer

	err := s.ServeStdio(context.Background(), in, &out, "1.0.0")
	require.NoError(t, err)

	var resp JSONRPCResponse
	require.NoError(t, json.Unmarshal(out.Bytes(), &resp))

	assert.Equal(t, float64(2), resp.ID)
	assert.Nil(t, resp.Error)

	result, ok := resp.Result.(map[string]any)
	require.True(t, ok)
	tools, ok := result["tools"].([]any)
	require.True(t, ok)
	assert.Len(t, tools, 6)
}

func TestServeStdio_ToolsCall(t *testing.T) {
	s := testServer(t)
	params := CallToolParams{
		Name:      "list_providers",
		Arguments: nil,
	}
	line := sendRequest(t, "tools/call", 3, params)

	in := strings.NewReader(line + "\n")
	var out bytes.Buffer

	err := s.ServeStdio(context.Background(), in, &out, "1.0.0")
	require.NoError(t, err)

	var resp JSONRPCResponse
	require.NoError(t, json.Unmarshal(out.Bytes(), &resp))

	assert.Equal(t, float64(3), resp.ID)
	assert.Nil(t, resp.Error)
}

func TestServeStdio_ToolsCall_InvalidParams(t *testing.T) {
	s := testServer(t)
	// Send raw invalid params
	req := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":"not-an-object"}`

	in := strings.NewReader(req + "\n")
	var out bytes.Buffer

	err := s.ServeStdio(context.Background(), in, &out, "1.0.0")
	require.NoError(t, err)

	var resp JSONRPCResponse
	require.NoError(t, json.Unmarshal(out.Bytes(), &resp))

	assert.NotNil(t, resp.Error)
	assert.Equal(t, -32602, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "invalid params")
}

func TestServeStdio_Ping(t *testing.T) {
	s := testServer(t)
	line := sendRequest(t, "ping", 5, nil)

	in := strings.NewReader(line + "\n")
	var out bytes.Buffer

	err := s.ServeStdio(context.Background(), in, &out, "1.0.0")
	require.NoError(t, err)

	var resp JSONRPCResponse
	require.NoError(t, json.Unmarshal(out.Bytes(), &resp))

	assert.Equal(t, float64(5), resp.ID)
	assert.Nil(t, resp.Error)
}

func TestServeStdio_UnknownMethod(t *testing.T) {
	s := testServer(t)
	line := sendRequest(t, "unknown/method", 6, nil)

	in := strings.NewReader(line + "\n")
	var out bytes.Buffer

	err := s.ServeStdio(context.Background(), in, &out, "1.0.0")
	require.NoError(t, err)

	var resp JSONRPCResponse
	require.NoError(t, json.Unmarshal(out.Bytes(), &resp))

	assert.NotNil(t, resp.Error)
	assert.Equal(t, -32601, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "method not found")
}

func TestServeStdio_Notification(t *testing.T) {
	s := testServer(t)
	line := sendRequest(t, "notifications/initialized", nil, nil)

	in := strings.NewReader(line + "\n")
	var out bytes.Buffer

	err := s.ServeStdio(context.Background(), in, &out, "1.0.0")
	require.NoError(t, err)

	// Notifications produce no response
	assert.Empty(t, out.String())
}

func TestServeStdio_InvalidJSON(t *testing.T) {
	s := testServer(t)
	in := strings.NewReader("not valid json\n")
	var out bytes.Buffer

	err := s.ServeStdio(context.Background(), in, &out, "1.0.0")
	require.NoError(t, err)

	// Invalid JSON is skipped, no output
	assert.Empty(t, out.String())
}

func TestServeStdio_EmptyLines(t *testing.T) {
	s := testServer(t)
	line := sendRequest(t, "ping", 7, nil)
	in := strings.NewReader("\n\n" + line + "\n\n")
	var out bytes.Buffer

	err := s.ServeStdio(context.Background(), in, &out, "1.0.0")
	require.NoError(t, err)

	var resp JSONRPCResponse
	require.NoError(t, json.Unmarshal(out.Bytes(), &resp))
	assert.Equal(t, float64(7), resp.ID)
}

func TestServeStdio_MultipleRequests(t *testing.T) {
	s := testServer(t)
	line1 := sendRequest(t, "ping", 10, nil)
	line2 := sendRequest(t, "ping", 11, nil)

	in := strings.NewReader(line1 + "\n" + line2 + "\n")
	var out bytes.Buffer

	err := s.ServeStdio(context.Background(), in, &out, "1.0.0")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	assert.Len(t, lines, 2)

	var resp1, resp2 JSONRPCResponse
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &resp1))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &resp2))
	assert.Equal(t, float64(10), resp1.ID)
	assert.Equal(t, float64(11), resp2.ID)
}

func TestServeStdio_ContextCancelled(t *testing.T) {
	s := testServer(t)

	// Use a reader that blocks until context is cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// The reader will have data but context is already cancelled
	line := sendRequest(t, "ping", 1, nil)
	in := strings.NewReader(line + "\n")
	var out bytes.Buffer

	// ServeStdio reads one line, checks context on next iteration
	_ = s.ServeStdio(ctx, in, &out, "1.0.0")
}
