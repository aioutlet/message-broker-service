package middleware

import (
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/aioutlet/message-broker-service/internal/logger"
)

// TracingConfig holds configuration for tracing middleware
type TracingConfig struct {
	ServiceName         string
	CorrelationIDHeader string
	Enabled             bool
}

// Tracing returns a middleware that creates spans for HTTP requests
func Tracing(cfg TracingConfig, log *logger.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !cfg.Enabled {
			return c.Next()
		}

		// Extract trace context from incoming request headers
		propagator := otel.GetTextMapPropagator()
		headers := make(map[string]string)
		
		// Convert Fiber headers to map for propagation
		c.Request().Header.VisitAll(func(key, value []byte) {
			headers[string(key)] = string(value)
		})
		
		parentCtx := propagator.Extract(c.Context(), propagation.MapCarrier(headers))

		// Start a new span for this HTTP request
		tracer := otel.Tracer(cfg.ServiceName)
		spanName := c.Method() + " " + c.Path()
		
		ctx, span := tracer.Start(parentCtx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPMethod(c.Method()),
				semconv.HTTPURL(c.OriginalURL()),
				semconv.HTTPScheme(c.Protocol()),
				semconv.HTTPRoute(c.Route().Path),
				attribute.String("http.user_agent", c.Get("User-Agent")),
				attribute.String("net.peer.ip", c.IP()),
				attribute.String("http.request_id", c.Get("X-Request-ID")),
			),
		)
		defer span.End()

		// Store span in context for use in handlers
		c.SetUserContext(ctx)

		// Add correlation ID if present
		if correlationID := c.Get(cfg.CorrelationIDHeader); correlationID != "" {
			span.SetAttributes(attribute.String("correlation.id", correlationID))
		}

		// Process request
		err := c.Next()

		// Set span status and attributes based on response
		statusCode := c.Response().StatusCode()
		span.SetAttributes(
			semconv.HTTPStatusCode(statusCode),
			attribute.Int("http.response.size", len(c.Response().Body())),
		)

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else if statusCode >= 400 {
			span.SetStatus(codes.Error, "HTTP error")
		} else {
			span.SetStatus(codes.Ok, "")
		}

		// Inject trace context into response headers for downstream services
		responseHeaders := make(map[string]string)
		propagator.Inject(ctx, propagation.MapCarrier(responseHeaders))
		
		for key, value := range responseHeaders {
			c.Response().Header.Set(key, value)
		}

		return err
	}
}