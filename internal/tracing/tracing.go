package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/aioutlet/message-broker-service/internal/config"
	"github.com/aioutlet/message-broker-service/internal/logger"
)

// TracerProvider holds the global tracer provider
var globalTracerProvider *sdktrace.TracerProvider

// InitTracing initializes OpenTelemetry distributed tracing
func InitTracing(cfg *config.Config, log *logger.Logger) (*sdktrace.TracerProvider, error) {
	if !cfg.Observability.EnableTracing {
		log.Info("Distributed tracing is disabled")
		// Return a no-op tracer provider
		return sdktrace.NewTracerProvider(), nil
	}

	log.Info("Initializing OpenTelemetry distributed tracing...",
		"service", cfg.ServiceName,
		"environment", cfg.Environment,
		"endpoint", cfg.Observability.OTLPExporterEndpoint)

	// Create OTLP HTTP exporter
	exporter, err := otlptracehttp.New(
		context.Background(),
		otlptracehttp.WithEndpoint(cfg.Observability.OTLPExporterEndpoint),
		otlptracehttp.WithInsecure(), // Use insecure for local development
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create resource with service information
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion("1.0.0"),
			semconv.DeploymentEnvironment(cfg.Environment),
			semconv.ServiceInstanceID(fmt.Sprintf("%s-%d", cfg.ServiceName, time.Now().Unix())),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create tracer provider with batch span processor
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // Sample all traces in development
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)
	globalTracerProvider = tp

	// Set global text map propagator to handle trace context propagation
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	log.Info("OpenTelemetry tracing initialized successfully",
		"service", cfg.ServiceName,
		"environment", cfg.Environment,
		"endpoint", cfg.Observability.OTLPExporterEndpoint)

	return tp, nil
}

// Shutdown gracefully shuts down the tracer provider
func Shutdown(ctx context.Context, log *logger.Logger) error {
	if globalTracerProvider == nil {
		return nil
	}

	log.Info("Shutting down OpenTelemetry tracer provider...")
	
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := globalTracerProvider.Shutdown(shutdownCtx); err != nil {
		log.Error("Failed to shutdown tracer provider", "error", err)
		return err
	}

	log.Info("OpenTelemetry tracer provider shutdown successfully")
	return nil
}

// GetTracer returns a tracer for the message broker service
func GetTracer() trace.Tracer {
	return otel.Tracer("message-broker-service")
}

// StartSpan starts a new span with the given name and context
func StartSpan(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tracer := GetTracer()
	return tracer.Start(ctx, spanName, opts...)
}

// SpanFromContext returns the span from the given context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// SetSpanStatus sets the status of a span
func SetSpanStatus(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
}

// AddSpanAttributes adds attributes to the current span
func AddSpanAttributes(ctx context.Context, attributes map[string]interface{}) {
	span := SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	for _, value := range attributes {
		switch v := value.(type) {
		case string:
			// Example: add as string attribute
			span.SetAttributes(semconv.ServiceName(v))
		case int:
			// Example: add as int attribute  
			span.SetAttributes(semconv.HTTPStatusCode(v))
		default:
			// Handle other types as needed
		}
	}
}