package handlers

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

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
	// Create a child span for health check
	_, span := otel.Tracer(h.config.ServiceName).Start(c.UserContext(), "health_check")
	defer span.End()

	isHealthy := h.broker.IsHealthy()
	status := "healthy"
	statusCode := fiber.StatusOK

	if !isHealthy {
		status = "unhealthy"
		statusCode = fiber.StatusServiceUnavailable
		span.SetAttributes(attribute.String("health.status", "unhealthy"))
		span.SetStatus(codes.Error, "broker unhealthy")
	} else {
		span.SetAttributes(attribute.String("health.status", "healthy"))
		span.SetStatus(codes.Ok, "")
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
	// Create a child span for message publishing
	ctx, span := otel.Tracer(h.config.ServiceName).Start(c.UserContext(), "publish_message",
		trace.WithSpanKind(trace.SpanKindProducer),
	)
	defer span.End()

	// Parse request (PublishRequest is alias for Message)
	var req models.PublishRequest
	if err := c.BodyParser(&req); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid request body")
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Success: false,
			Error:   "invalid request body",
			Code:    "INVALID_REQUEST",
		})
	}

	// Validate required AWS EventBridge fields
	if req.EventType == "" {
		span.SetStatus(codes.Error, "missing eventType")
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Success: false,
			Error:   "eventType is required",
			Code:    "MISSING_EVENT_TYPE",
		})
	}

	if req.Source == "" {
		span.SetStatus(codes.Error, "missing source")
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Success: false,
			Error:   "source is required",
			Code:    "MISSING_SOURCE",
		})
	}

	if req.Data == nil || len(req.Data) == 0 {
		span.SetStatus(codes.Error, "missing data")
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Success: false,
			Error:   "data is required",
			Code:    "MISSING_DATA",
		})
	}

	// Generate broker-specific ID for this message
	req.ID = ""  // Will be set by NewMessage
	message := models.NewMessage(
		req.Source,
		req.EventType,
		req.EventVersion,
		req.EventID,
		req.Timestamp.Format(time.RFC3339),
		req.Data,
		req.Metadata,
	)
	message.CorrelationID = req.CorrelationID
	message.Priority = req.Priority

	// Add request attributes to span
	span.SetAttributes(
		attribute.String("message.event_type", message.EventType),
		attribute.String("message.source", message.Source),
		attribute.String("message.event_version", message.EventVersion),
		attribute.String("message.event_id", message.EventID),
		attribute.String("message.correlation_id", message.CorrelationID),
		attribute.Int("message.priority", message.Priority),
		attribute.Int("message.data_size", len(fmt.Sprintf("%v", message.Data))),
	)

	// Debug: Log incoming request details  
	correlationHeader := h.config.Observability.CorrelationIDHeader
	h.log.Infof("📨 Incoming publish request: eventType=%s | source=%s | version=%s | correlationId=%s | xRequestId=%s | correlationFromHeader=%s",
		message.EventType,
		message.Source,
		message.EventVersion,
		message.CorrelationID,
		c.Get("X-Request-ID"),
		c.Get(correlationHeader),
	)

	// Add request ID as correlation ID if not provided
	if message.CorrelationID == "" {
		message.CorrelationID = c.Get("X-Request-ID", "")
	}

	// Try configured correlation ID header
	if message.CorrelationID == "" {
		message.CorrelationID = c.Get(correlationHeader, "")
	}

	// Inject trace context into message headers for downstream propagation
	propagator := otel.GetTextMapPropagator()
	traceHeaders := make(map[string]string)
	propagator.Inject(ctx, propagation.MapCarrier(traceHeaders))
	message.SetTraceHeaders(traceHeaders)

	// Add final message attributes to span
	span.SetAttributes(
		attribute.String("message.id", message.ID),
		attribute.String("message.final_correlation_id", message.CorrelationID),
		attribute.String("message.timestamp", message.Timestamp.Format(time.RFC3339)),
	)

	// Publish message with trace context
	if err := h.broker.Publish(ctx, message); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to publish message")
		
		h.log.Errorf("Failed to publish message: error=%s | eventType=%s | messageId=%s | correlationId=%s",
			err.Error(),
			message.EventType,
			message.ID,
			message.CorrelationID,
		)

		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Success: false,
			Error:   "failed to publish message",
			Code:    "PUBLISH_FAILED",
		})
	}

	// Mark span as successful
	span.SetStatus(codes.Ok, "message published successfully")

	h.log.Infof("✅ Message published to broker: messageId=%s | eventType=%s | source=%s | version=%s | correlationId=%s | dataSize=%d",
		message.ID,
		message.EventType,
		message.Source,
		message.EventVersion,
		message.CorrelationID,
		len(fmt.Sprintf("%v", message.Data)),
	)

	// Return response
	return c.JSON(models.PublishResponse{
		Success:   true,
		MessageID: message.ID,
		EventType: message.EventType,
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
