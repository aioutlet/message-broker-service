package handlers

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/aioutlet/message-broker-service/internal/broker"
	"github.com/aioutlet/message-broker-service/internal/config"
	"github.com/aioutlet/message-broker-service/internal/logger"
	"github.com/aioutlet/message-broker-service/internal/models"
)

// Handler handles HTTP requests
type Handler struct {
	broker *broker.Manager
	config *config.Config
	log    *logger.Logger
}

// NewHandler creates a new handler instance
func NewHandler(broker *broker.Manager, cfg *config.Config, log *logger.Logger) *Handler {
	return &Handler{
		broker: broker,
		config: cfg,
		log:    log,
	}
}

// Health handles health check requests
// @Summary Health check
// @Description Check if service and broker are healthy
// @Tags health
// @Produce json
// @Success 200 {object} models.HealthResponse
// @Router /api/v1/health [get]
func (h *Handler) Health(c *fiber.Ctx) error {
	isHealthy := h.broker.IsHealthy()
	status := "healthy"
	statusCode := fiber.StatusOK

	if !isHealthy {
		status = "unhealthy"
		statusCode = fiber.StatusServiceUnavailable
	}

	return c.Status(statusCode).JSON(models.HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Broker:    h.broker.GetStats(),
	})
}

// Publish handles message publishing requests
// @Summary Publish a message
// @Description Publish a message to the message broker
// @Tags messages
// @Accept json
// @Produce json
// @Param request body models.PublishRequest true "Publish request"
// @Success 200 {object} models.PublishResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/publish [post]
func (h *Handler) Publish(c *fiber.Ctx) error {
	// Parse request
	var req models.PublishRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Success: false,
			Error:   "invalid request body",
			Code:    "INVALID_REQUEST",
		})
	}

	// Validate request
	if req.Topic == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Success: false,
			Error:   "topic is required",
			Code:    "MISSING_TOPIC",
		})
	}

	if req.Data == nil || len(req.Data) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Success: false,
			Error:   "data is required",
			Code:    "MISSING_DATA",
		})
	}

	// Get service name from header or use default
	serviceName := c.Get("X-Service-Name", "unknown")

	// Debug: Log incoming request details  
	correlationHeader := h.config.Observability.CorrelationIDHeader
	h.log.Infof("📨 Incoming publish request: topic=%s | correlationIdFromRequest=%s | xRequestId=%s | correlationFromHeader=%s | serviceName=%s | correlationHeader=%s",
		req.Topic,
		req.CorrelationID,
		c.Get("X-Request-ID"),
		c.Get(correlationHeader),
		serviceName,
		correlationHeader,
	)

	// Create message
	message := models.NewMessage(req.Topic, req.Data, serviceName)
	message.CorrelationID = req.CorrelationID
	message.Metadata.Priority = req.Priority

	// Add request ID as correlation ID if not provided
	if message.CorrelationID == "" {
		message.CorrelationID = c.Get("X-Request-ID", "")
	}

	// Try configured correlation ID header
	if message.CorrelationID == "" {
		message.CorrelationID = c.Get(correlationHeader, "")
	}

	// Final fallback - extract from event data
	if message.CorrelationID == "" {
		if metadata, ok := req.Data["metadata"].(map[string]interface{}); ok {
			if corrId, ok := metadata["correlationId"].(string); ok {
				message.CorrelationID = corrId
			}
		}
	}

	// Publish message
	ctx := c.Context()
	if err := h.broker.Publish(ctx, message); err != nil {
		h.log.Errorf("Failed to publish message: error=%s | topic=%s | messageId=%s | correlationId=%s",
			err.Error(),
			req.Topic,
			message.ID,
			message.CorrelationID,
		)

		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Success: false,
			Error:   "failed to publish message",
			Code:    "PUBLISH_FAILED",
		})
	}

	h.log.Infof("✅ Message published to broker: messageId=%s | topic=%s | source=%s | correlationId=%s | eventType=%v | dataSize=%d",
		message.ID,
		req.Topic,
		serviceName,
		message.CorrelationID,
		message.Data["eventType"],
		len(fmt.Sprintf("%v", req.Data)),
	)

	// Return response
	return c.JSON(models.PublishResponse{
		Success:   true,
		MessageID: message.ID,
		Topic:     req.Topic,
		Timestamp: message.Timestamp,
	})
}

// Stats handles statistics requests
// @Summary Get broker statistics
// @Description Get statistics about the message broker
// @Tags stats
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/stats [get]
func (h *Handler) Stats(c *fiber.Ctx) error {
	stats := h.broker.GetStats()
	stats["service_name"] = h.config.ServiceName
	stats["environment"] = h.config.Environment
	stats["uptime"] = time.Since(time.Now()).String() // You'd track actual startup time

	return c.JSON(fiber.Map{
		"success": true,
		"data":    stats,
	})
}
