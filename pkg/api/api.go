// Package api provides the HTTP server for event-reactor, including
// health endpoints, webhook/CloudEvents listeners, and metrics.
package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/oakwood-commons/event-reactor/pkg/adapter"
	"github.com/oakwood-commons/event-reactor/pkg/config"
	"github.com/oakwood-commons/event-reactor/pkg/message"
)

const (
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 30 * time.Second
)

// Server is the event-reactor HTTP server.
type Server struct {
	cfg     *config.ServerConfig
	adapter *adapter.Adapter
	logger  *slog.Logger
	engine  *gin.Engine
}

// New creates a Server with health and webhook endpoints.
func New(cfg *config.ServerConfig, a *adapter.Adapter, logger *slog.Logger) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	s := &Server{
		cfg:     cfg,
		adapter: a,
		logger:  logger,
		engine:  engine,
	}

	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.engine.GET(s.cfg.Server.HealthCheck.Liveness, s.handleLiveness)
	s.engine.GET(s.cfg.Server.HealthCheck.Readiness, s.handleReadiness)
	s.engine.POST("/events", s.handleEvent)
	s.engine.POST("/cloudevents", s.handleCloudEvent)
	s.engine.POST("/webhook/:source", s.handleWebhook)
}

// Router returns the underlying gin.Engine for testing.
func (s *Server) Router() *gin.Engine {
	return s.engine
}

// Start starts the HTTP server and blocks until the context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.cfg.Server.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.engine,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("starting HTTP server", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		s.logger.Info("shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleLiveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleReadiness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func (s *Server) handleEvent(c *gin.Context) {
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload"})
		return
	}

	event, err := message.FromGenericPayload(payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid event: %v", err)})
		return
	}

	results := s.adapter.HandleEvent(c.Request.Context(), event)

	c.JSON(http.StatusOK, gin.H{
		"processed": len(results),
		"results":   results,
	})
}

// handleCloudEvent handles CloudEvents in structured (JSON) or binary content mode.
func (s *Server) handleCloudEvent(c *gin.Context) {
	// Check for binary content mode (attributes in headers)
	ceType := c.GetHeader("Ce-Type")
	ceSource := c.GetHeader("Ce-Source")
	ceID := c.GetHeader("Ce-Id")

	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload"})
		return
	}

	if ceType != "" {
		// Binary content mode: headers carry metadata, body is data
		event := message.Event{
			ID:     ceID,
			Source: ceSource,
			Type:   ceType,
			Attributes: map[string]string{
				"specversion": c.GetHeader("Ce-Specversion"),
				"source":      ceSource,
				"type":        ceType,
			},
			Payload: payload,
		}
		if event.ID == "" {
			event.ID = uuid.New().String()
		}
		results := s.adapter.HandleEvent(c.Request.Context(), event)
		c.JSON(http.StatusOK, gin.H{"processed": len(results), "results": results})
		return
	}

	// Structured content mode: full CloudEvent in body
	event := message.Event{
		Attributes: make(map[string]string),
		Payload:    payload["data"],
	}

	if id, ok := payload["id"].(string); ok {
		event.ID = id
	} else {
		event.ID = uuid.New().String()
	}
	if src, ok := payload["source"].(string); ok {
		event.Source = src
		event.Attributes["source"] = src
	}
	if t, ok := payload["type"].(string); ok {
		event.Type = t
		event.Attributes["type"] = t
	}
	if sv, ok := payload["specversion"].(string); ok {
		event.Attributes["specversion"] = sv
	}

	// If no data field, treat entire body as payload
	if event.Payload == nil {
		event.Payload = payload
	}

	results := s.adapter.HandleEvent(c.Request.Context(), event)
	c.JSON(http.StatusOK, gin.H{"processed": len(results), "results": results})
}

// handleWebhook handles generic webhook requests with optional HMAC validation.
func (s *Server) handleWebhook(c *gin.Context) {
	source := c.Param("source")

	// Read body for HMAC validation
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	// HMAC validation if configured
	if secret := s.webhookSecret(source); secret != "" {
		sig := c.GetHeader("X-Hub-Signature-256")
		if sig == "" {
			sig = c.GetHeader("X-Signature-256")
		}
		if !validateHMAC(body, sig, secret) {
			s.logger.Warn("webhook HMAC validation failed", slog.String("source", source))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload"})
		return
	}

	event := message.Event{
		ID:     uuid.New().String(),
		Source: source,
		Type:   c.GetHeader("X-Event-Type"),
		Attributes: map[string]string{
			"source":       source,
			"content-type": c.ContentType(),
		},
		Payload: payload,
	}

	// Add event type from common headers
	if event.Type == "" {
		event.Type = c.GetHeader("X-GitHub-Event")
	}
	if event.Type != "" {
		event.Attributes["type"] = event.Type
	}

	results := s.adapter.HandleEvent(c.Request.Context(), event)
	c.JSON(http.StatusOK, gin.H{"processed": len(results), "results": results})
}

func (s *Server) webhookSecret(source string) string {
	for _, wh := range s.cfg.Auth.WebhookSecrets {
		if wh.Source == source {
			return wh.Secret
		}
	}
	return ""
}

func validateHMAC(body []byte, signature, secret string) bool {
	if signature == "" {
		return false
	}
	sig := strings.TrimPrefix(signature, "sha256=")
	expected, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), expected)
}
