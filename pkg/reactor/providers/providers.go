// Package providers contains built-in reactor provider implementations.
package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/oakwood-commons/event-reactor/pkg/message"
	"github.com/oakwood-commons/event-reactor/pkg/reactor"
)

const httpTimeout = 30 * time.Second

// Echo logs inputs and returns them as output. Useful for testing.
type Echo struct{}

func (Echo) Name() string { return "echo" }

func (Echo) Execute(_ context.Context, inputs map[string]any, _ message.Event) (*reactor.Result, error) {
	return &reactor.Result{Provider: "echo", Output: inputs}, nil
}

// HTTP sends an HTTP request.
type HTTP struct {
	Client *http.Client
}

func NewHTTP() *HTTP {
	return &HTTP{Client: &http.Client{Timeout: httpTimeout}}
}

func (h *HTTP) Name() string { return "http" }

func (h *HTTP) Execute(ctx context.Context, inputs map[string]any, _ message.Event) (*reactor.Result, error) {
	method, _ := inputs["method"].(string)
	if method == "" {
		method = http.MethodPost
	}
	url, _ := inputs["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("http provider: url is required")
	}

	var body io.Reader
	if b, ok := inputs["body"]; ok {
		switch v := b.(type) {
		case string:
			body = strings.NewReader(v)
		default:
			data, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("http provider: marshaling body: %w", err)
			}
			body = bytes.NewReader(data)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("http provider: creating request: %w", err)
	}

	if ct, ok := inputs["contentType"].(string); ok {
		req.Header.Set("Content-Type", ct)
	} else if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if headers, ok := inputs["headers"].(map[string]any); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprint(v))
		}
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http provider: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	output := map[string]any{
		"statusCode": resp.StatusCode,
		"body":       string(respBody),
	}

	if resp.StatusCode >= 400 {
		return &reactor.Result{Provider: "http", Output: output},
			fmt.Errorf("http provider: status %d", resp.StatusCode)
	}

	return &reactor.Result{Provider: "http", Output: output}, nil
}

// Exec runs a local command.
type Exec struct {
	Logger *slog.Logger
}

func (e *Exec) Name() string { return "exec" }

func (e *Exec) Execute(ctx context.Context, inputs map[string]any, _ message.Event) (*reactor.Result, error) {
	command, _ := inputs["command"].(string)
	if command == "" {
		return nil, fmt.Errorf("exec provider: command is required")
	}

	args := []string{}
	if a, ok := inputs["args"].([]any); ok {
		for _, v := range a {
			args = append(args, fmt.Sprint(v))
		}
	}

	cmd := exec.CommandContext(ctx, command, args...)

	if dir, ok := inputs["dir"].(string); ok {
		cmd.Dir = dir
	}

	if stdin, ok := inputs["stdin"].(string); ok {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := map[string]any{
		"stdout":   stdout.String(),
		"stderr":   stderr.String(),
		"exitCode": cmd.ProcessState.ExitCode(),
	}

	if err != nil {
		return &reactor.Result{Provider: "exec", Output: output},
			fmt.Errorf("exec provider: %w", err)
	}

	return &reactor.Result{Provider: "exec", Output: output}, nil
}

// Log logs the event/inputs to the server logger.
type Log struct {
	Logger *slog.Logger
}

func (l *Log) Name() string { return "log" }

func (l *Log) Execute(_ context.Context, inputs map[string]any, event message.Event) (*reactor.Result, error) {
	level, _ := inputs["level"].(string)
	msg, _ := inputs["message"].(string)
	if msg == "" {
		msg = fmt.Sprintf("event %s from %s", event.ID, event.Source)
	}

	switch strings.ToLower(level) {
	case "error":
		l.Logger.Error(msg, slog.Any("inputs", inputs))
	case "warn", "warning":
		l.Logger.Warn(msg, slog.Any("inputs", inputs))
	case "debug":
		l.Logger.Debug(msg, slog.Any("inputs", inputs))
	default:
		l.Logger.Info(msg, slog.Any("inputs", inputs))
	}

	return &reactor.Result{Provider: "log", Output: map[string]any{"logged": true}}, nil
}

// RegisterAll registers all built-in providers in the registry.
func RegisterAll(reg *reactor.Registry, logger *slog.Logger) {
	reg.Register(Echo{})
	reg.Register(NewHTTP())
	reg.Register(&Exec{Logger: logger})
	reg.Register(&Log{Logger: logger})
}
