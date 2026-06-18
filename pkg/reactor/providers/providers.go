// Package providers contains built-in reactor provider implementations.
package providers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/smtp"
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

	// Auth token injected via context by the adapter when reactor references an auth handler
	if authHeader, ok := reactor.AuthHeader(ctx); ok {
		req.Header.Set("Authorization", authHeader)
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
	reg.Register(&SMTP{Logger: logger})
}

// --- SMTP provider ---

const (
	smtpDefaultPort  = "587"
	smtpDialTimeout  = 30 * time.Second
	smtpRetryBase    = 1 * time.Second
	smtpRetryMaxWait = 30 * time.Second
)

// SMTPDialer abstracts SMTP connection creation for testability.
type SMTPDialer interface {
	Dial(addr string) (SMTPClient, error)
}

// SMTPClient abstracts the net/smtp.Client methods used by the provider.
type SMTPClient interface {
	StartTLS(config *tls.Config) error
	Auth(a smtp.Auth) error
	Mail(from string) error
	Rcpt(to string) error
	Data() (io.WriteCloser, error)
	Quit() error
	Close() error
}

// SMTP sends email via SMTP.
type SMTP struct {
	Logger   *slog.Logger
	Dialer   SMTPDialer                      // nil uses default net dialer
	NewTimer func(time.Duration) *time.Timer // nil uses time.NewTimer; injectable for tests
}

func (s *SMTP) Name() string { return "smtp" }

func (s *SMTP) Execute(ctx context.Context, inputs map[string]any, _ message.Event) (*reactor.Result, error) {
	params, err := parseSMTPInput(inputs)
	if err != nil {
		return nil, fmt.Errorf("smtp provider: %w", err)
	}

	// Auth token from context can serve as SMTP password when auth handler is configured.
	// The header value may contain a scheme prefix (e.g. "Bearer <token>") — extract
	// the credential portion after the first space, or use the full string if no scheme.
	if params.password == "" {
		if authHeader, ok := reactor.AuthHeader(ctx); ok {
			params.password = authHeader
			if parts := strings.SplitN(authHeader, " ", 2); len(parts) == 2 {
				params.password = parts[1]
			}
		}
	}

	attempts, err := s.sendMail(ctx, params)
	output := map[string]any{
		"to":       params.to,
		"subject":  params.subject,
		"attempts": attempts,
	}

	if err != nil {
		return &reactor.Result{Provider: "smtp", Output: output},
			fmt.Errorf("smtp provider: %w", err)
	}

	return &reactor.Result{Provider: "smtp", Output: output}, nil
}

type smtpParams struct {
	host        string
	port        string
	from        string
	to          []string
	cc          []string
	bcc         []string
	subject     string
	body        string
	contentType string
	username    string
	password    string
	startTLS    bool
	maxRetries  int
}

func parseSMTPInput(input map[string]any) (*smtpParams, error) {
	host, _ := input["host"].(string)
	if host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if smtpContainsCRLF(host) {
		return nil, fmt.Errorf("host contains invalid characters")
	}

	from, _ := input["from"].(string)
	if from == "" {
		return nil, fmt.Errorf("from is required")
	}
	if smtpContainsCRLF(from) {
		return nil, fmt.Errorf("from contains invalid characters")
	}

	to := smtpExtractRecipients(input["to"])
	if len(to) == 0 {
		return nil, fmt.Errorf("to is required (at least one recipient)")
	}
	for _, addr := range to {
		if smtpContainsCRLF(addr) {
			return nil, fmt.Errorf("to address contains invalid characters")
		}
	}

	subject, _ := input["subject"].(string)
	if subject == "" {
		return nil, fmt.Errorf("subject is required")
	}
	if smtpContainsCRLF(subject) {
		return nil, fmt.Errorf("subject contains invalid characters")
	}

	body, _ := input["body"].(string)
	if body == "" {
		return nil, fmt.Errorf("body is required")
	}

	port := smtpExtractPort(input["port"])

	contentType, _ := input["contentType"].(string)
	if contentType == "" {
		contentType = "text/plain"
	}
	if smtpContainsCRLF(contentType) {
		return nil, fmt.Errorf("contentType contains invalid characters")
	}

	username, _ := input["username"].(string)
	password, _ := input["password"].(string)

	startTLS := username != ""
	if v, ok := input["startTLS"].(bool); ok {
		startTLS = v
	} else if v, ok := input["starttls"].(bool); ok {
		startTLS = v
	}

	maxRetries := smtpExtractInt(input["maxRetries"])
	if maxRetries < 0 {
		maxRetries = 0
	}
	if maxRetries > 10 {
		maxRetries = 10
	}

	cc := smtpExtractRecipients(input["cc"])
	for _, addr := range cc {
		if smtpContainsCRLF(addr) {
			return nil, fmt.Errorf("cc address contains invalid characters")
		}
	}

	bcc := smtpExtractRecipients(input["bcc"])
	for _, addr := range bcc {
		if smtpContainsCRLF(addr) {
			return nil, fmt.Errorf("bcc address contains invalid characters")
		}
	}

	return &smtpParams{
		host:        host,
		port:        port,
		from:        from,
		to:          to,
		cc:          cc,
		bcc:         bcc,
		subject:     subject,
		body:        body,
		contentType: contentType,
		username:    username,
		password:    password,
		startTLS:    startTLS,
		maxRetries:  maxRetries,
	}, nil
}

func (s *SMTP) sendMail(ctx context.Context, params *smtpParams) (int, error) {
	var lastErr error
	attempts := 0

	for attempt := 0; attempt <= params.maxRetries; attempt++ {
		attempts = attempt + 1

		if attempt > 0 {
			exp := float64(attempt - 1)
			if exp > 30 {
				exp = 30 // cap to prevent math.Pow overflow
			}
			delay := smtpRetryBase * time.Duration(math.Pow(2, exp))
			if delay > smtpRetryMaxWait {
				delay = smtpRetryMaxWait
			}
			newTimer := s.NewTimer
			if newTimer == nil {
				newTimer = time.NewTimer
			}
			timer := newTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return attempts, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			case <-timer.C:
			}
		}

		err := s.trySend(ctx, params)
		if err == nil {
			return attempts, nil
		}
		lastErr = err

		if !smtpIsTransient(err) {
			return attempts, err
		}

		s.Logger.Warn("smtp: transient error, retrying",
			slog.Int("attempt", attempts),
			slog.String("error", err.Error()),
		)
	}

	return attempts, lastErr
}

func (s *SMTP) trySend(ctx context.Context, params *smtpParams) error {
	addr := net.JoinHostPort(params.host, params.port)

	var client SMTPClient
	var err error

	if s.Dialer != nil {
		client, err = s.Dialer.Dial(addr)
	} else {
		var conn net.Conn
		dialer := &net.Dialer{Timeout: smtpDialTimeout}
		conn, err = dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("dialing SMTP server: %w", err)
		}
		c, cErr := smtp.NewClient(conn, params.host)
		if cErr != nil {
			conn.Close()
			return fmt.Errorf("creating SMTP client: %w", cErr)
		}
		client = &smtpClientWrapper{c: c}
	}
	if err != nil {
		return fmt.Errorf("dialing SMTP server: %w", err)
	}
	defer client.Close()

	if params.startTLS {
		tlsCfg := &tls.Config{ServerName: params.host, MinVersion: tls.VersionTLS12}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("starting TLS: %w", err)
		}
	}

	if params.username != "" && params.password != "" {
		auth := smtp.PlainAuth("", params.username, params.password, params.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authenticating: %w", err)
		}
	}

	if err := client.Mail(params.from); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}

	allRecipients := make([]string, 0, len(params.to)+len(params.cc)+len(params.bcc))
	allRecipients = append(allRecipients, params.to...)
	allRecipients = append(allRecipients, params.cc...)
	allRecipients = append(allRecipients, params.bcc...)
	for _, rcpt := range allRecipients {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("RCPT TO %s: %w", rcpt, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}

	msg := smtpBuildMessage(params)
	if _, err := w.Write(msg); err != nil {
		w.Close()
		return fmt.Errorf("writing message: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("closing message: %w", err)
	}

	return client.Quit()
}

// smtpClientWrapper adapts net/smtp.Client to the SMTPClient interface.
type smtpClientWrapper struct {
	c *smtp.Client
}

func (w *smtpClientWrapper) StartTLS(config *tls.Config) error { return w.c.StartTLS(config) }
func (w *smtpClientWrapper) Auth(a smtp.Auth) error            { return w.c.Auth(a) }
func (w *smtpClientWrapper) Mail(from string) error            { return w.c.Mail(from) }
func (w *smtpClientWrapper) Rcpt(to string) error              { return w.c.Rcpt(to) }
func (w *smtpClientWrapper) Data() (io.WriteCloser, error)     { return w.c.Data() }
func (w *smtpClientWrapper) Quit() error                       { return w.c.Quit() }
func (w *smtpClientWrapper) Close() error                      { return w.c.Close() }

func smtpBuildMessage(params *smtpParams) []byte {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&sb, "Message-ID: <%d.%s>\r\n", time.Now().UnixNano(), params.from)
	fmt.Fprintf(&sb, "From: %s\r\n", params.from)
	fmt.Fprintf(&sb, "To: %s\r\n", strings.Join(params.to, ", "))
	if len(params.cc) > 0 {
		fmt.Fprintf(&sb, "Cc: %s\r\n", strings.Join(params.cc, ", "))
	}
	fmt.Fprintf(&sb, "Subject: %s\r\n", params.subject)
	fmt.Fprintf(&sb, "Content-Type: %s; charset=UTF-8\r\n", params.contentType)
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(params.body)
	return []byte(sb.String())
}

func smtpExtractRecipients(input any) []string {
	if input == nil {
		return nil
	}
	switch v := input.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s != "" {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

func smtpExtractPort(input any) string {
	switch v := input.(type) {
	case string:
		if v != "" {
			return v
		}
	case int:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%d", int(v))
	}
	return smtpDefaultPort
}

func smtpExtractInt(input any) int {
	switch v := input.(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

func smtpContainsCRLF(s string) bool {
	return strings.ContainsAny(s, "\r\n")
}

func smtpIsTransient(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()

	if strings.Contains(msg, "authenticating") {
		return false
	}
	if strings.Contains(msg, "dialing SMTP server") {
		return true
	}
	if strings.Contains(msg, "starting TLS") {
		return true
	}

	var netErr net.Error
	return errors.As(err, &netErr)
}
