package adapters

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/aioutlet/message-broker-service/internal/config"
	"github.com/aioutlet/message-broker-service/internal/logger"
	"github.com/aioutlet/message-broker-service/internal/models"
)

// RabbitMQAdapter implements the Broker interface for RabbitMQ
type RabbitMQAdapter struct {
	config     *config.RabbitMQConfig
	log        *logger.Logger
	conn       *amqp.Connection
	channel    *amqp.Channel
	connected  bool
	mu         sync.RWMutex
	stats      Stats
}

// Stats holds statistics for the adapter
type Stats struct {
	MessagesPublished uint64
	MessagesFailed    uint64
	LastPublishedAt   time.Time
	mu                sync.RWMutex
}

// NewRabbitMQAdapter creates a new RabbitMQ adapter
func NewRabbitMQAdapter(cfg *config.RabbitMQConfig, log *logger.Logger) (*RabbitMQAdapter, error) {
	return &RabbitMQAdapter{
		config: cfg,
		log:    log.WithFields(map[string]interface{}{"adapter": "rabbitmq"}),
	}, nil
}

// Connect establishes connection to RabbitMQ
func (r *RabbitMQAdapter) Connect(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.log.Info("Connecting to RabbitMQ", "url", maskPassword(r.config.URL))

	// Establish connection
	conn, err := amqp.Dial(r.config.URL)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	// Create channel
	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare exchange
	err = channel.ExchangeDeclare(
		r.config.Exchange,     // name
		r.config.ExchangeType, // type
		true,                  // durable
		false,                 // auto-deleted
		false,                 // internal
		false,                 // no-wait
		nil,                   // arguments
	)
	if err != nil {
		channel.Close()
		conn.Close()
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	r.conn = conn
	r.channel = channel
	r.connected = true

	// Setup connection close handler
	go r.handleConnectionErrors()

	r.log.Info("Successfully connected to RabbitMQ",
		"exchange", r.config.Exchange,
		"exchangeType", r.config.ExchangeType,
	)

	return nil
}

// Disconnect closes the RabbitMQ connection
func (r *RabbitMQAdapter) Disconnect(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.connected {
		return nil
	}

	r.log.Info("Disconnecting from RabbitMQ")

	if r.channel != nil {
		if err := r.channel.Close(); err != nil {
			r.log.Warn("Error closing channel", "error", err)
		}
	}

	if r.conn != nil {
		if err := r.conn.Close(); err != nil {
			r.log.Warn("Error closing connection", "error", err)
		}
	}

	r.connected = false
	return nil
}

// Publish publishes a message to RabbitMQ with distributed tracing support
func (r *RabbitMQAdapter) Publish(ctx context.Context, message *models.Message) error {
	// Create a span for RabbitMQ publishing
	tracer := otel.Tracer("message-broker-service.rabbitmq")
	ctx, span := tracer.Start(ctx, "rabbitmq.publish",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", message.EventType),
			attribute.String("messaging.destination_kind", "topic"),
			attribute.String("messaging.rabbitmq.routing_key", message.EventType),
			attribute.String("message.id", message.ID),
			attribute.String("message.correlation_id", message.CorrelationID),
		),
	)
	defer span.End()

	r.mu.RLock()
	if !r.connected || r.channel == nil {
		r.mu.RUnlock()
		span.SetStatus(codes.Error, "broker not connected")
		return models.ErrBrokerNotConnected
	}
	channel := r.channel
	r.mu.RUnlock()

	// Validate message
	if err := message.Validate(); err != nil {
		r.updateStats(false)
		span.RecordError(err)
		span.SetStatus(codes.Error, "message validation failed")
		return err
	}

	// Inject trace context into message headers for downstream propagation
	propagator := otel.GetTextMapPropagator()
	traceHeaders := make(map[string]string)
	propagator.Inject(ctx, propagation.MapCarrier(traceHeaders))
	message.SetTraceHeaders(traceHeaders)

	// Convert message to JSON
	body, err := message.ToJSON()
	if err != nil {
		r.updateStats(false)
		span.RecordError(err)
		span.SetStatus(codes.Error, "message serialization failed")
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Prepare publishing with trace headers
	publishing := amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent, // Make message persistent
		Timestamp:    message.Timestamp,
		MessageId:    message.ID,
		Priority:     uint8(message.Priority),
		Headers:      amqp.Table{
			"source":       message.Source,
			"eventType":    message.EventType,
			"eventVersion": message.EventVersion,
			"eventId":      message.EventID,
		},
	}

	// Add correlation ID if present
	if message.CorrelationID != "" {
		publishing.CorrelationId = message.CorrelationID
	}

	// Add trace headers
	for k, v := range message.TraceHeaders {
		publishing.Headers[k] = v
	}

	// Add span attributes for message size
	span.SetAttributes(attribute.Int("message.body.size", len(body)))

	// Publish with context and tracing
	err = channel.PublishWithContext(
		ctx,
		r.config.Exchange, // exchange
		message.GetRoutingKey(), // routing key
		false,             // mandatory
		false,             // immediate
		publishing,
	)

	if err != nil {
		r.updateStats(false)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to publish to RabbitMQ")
		return fmt.Errorf("failed to publish message: %w", err)
	}

	// Mark span as successful
	span.SetStatus(codes.Ok, "message published successfully")
	span.SetAttributes(
		attribute.String("rabbitmq.exchange", r.config.Exchange),
		attribute.Bool("message.persistent", true),
	)

	r.updateStats(true)
	r.log.Debug("Message published to RabbitMQ",
		"messageId", message.ID,
		"eventType", message.EventType,
		"exchange", r.config.Exchange,
		"correlationId", message.CorrelationID,
	)

	return nil
}

// Subscribe subscribes to messages from a topic
func (r *RabbitMQAdapter) Subscribe(ctx context.Context, topic string, handler func(*models.Message) error) error {
	r.mu.RLock()
	if !r.connected || r.channel == nil {
		r.mu.RUnlock()
		return models.ErrBrokerNotConnected
	}
	channel := r.channel
	r.mu.RUnlock()

	// Declare a queue (server generates name)
	queue, err := channel.QueueDeclare(
		"",    // name (empty = server generates)
		false, // durable
		true,  // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	// Bind queue to exchange with routing key
	err = channel.QueueBind(
		queue.Name,        // queue name
		topic,             // routing key
		r.config.Exchange, // exchange
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind queue: %w", err)
	}

	// Start consuming
	msgs, err := channel.Consume(
		queue.Name, // queue
		"",         // consumer
		false,      // auto-ack
		false,      // exclusive
		false,      // no-local
		false,      // no-wait
		nil,        // args
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	// Process messages in a goroutine
	go func() {
		for msg := range msgs {
			// Parse message
			message, err := models.FromJSON(msg.Body)
			if err != nil {
				r.log.Error("Failed to parse message", "error", err)
				msg.Nack(false, false) // Don't requeue
				continue
			}

			// Call handler
			if err := handler(message); err != nil {
				r.log.Error("Handler error", "error", err, "messageId", message.ID)
				msg.Nack(false, true) // Requeue
				continue
			}

			// Acknowledge message
			msg.Ack(false)
		}
	}()

	r.log.Info("Subscribed to topic", "topic", topic, "queue", queue.Name)
	return nil
}

// IsHealthy checks if the connection is healthy
func (r *RabbitMQAdapter) IsHealthy() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connected && r.conn != nil && !r.conn.IsClosed()
}

// GetStats returns adapter statistics
func (r *RabbitMQAdapter) GetStats() map[string]interface{} {
	r.stats.mu.RLock()
	defer r.stats.mu.RUnlock()

	return map[string]interface{}{
		"messages_published": r.stats.MessagesPublished,
		"messages_failed":    r.stats.MessagesFailed,
		"last_published_at":  r.stats.LastPublishedAt,
		"connected":          r.IsHealthy(),
	}
}

// handleConnectionErrors monitors connection errors
func (r *RabbitMQAdapter) handleConnectionErrors() {
	notifyClose := r.conn.NotifyClose(make(chan *amqp.Error))
	err := <-notifyClose
	if err != nil {
		r.log.Error("RabbitMQ connection closed", "error", err)
		r.mu.Lock()
		r.connected = false
		r.mu.Unlock()
	}
}

// updateStats updates adapter statistics
func (r *RabbitMQAdapter) updateStats(success bool) {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()

	if success {
		r.stats.MessagesPublished++
		r.stats.LastPublishedAt = time.Now()
	} else {
		r.stats.MessagesFailed++
	}
}

// maskPassword masks the password in the URL for logging
func maskPassword(url string) string {
	// Simple implementation - you can make this more robust
	return "amqp://****:****@..."
}
