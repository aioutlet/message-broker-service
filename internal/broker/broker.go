package broker

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/aioutlet/message-broker-service/internal/broker/adapters"
	"github.com/aioutlet/message-broker-service/internal/config"
	"github.com/aioutlet/message-broker-service/internal/logger"
	"github.com/aioutlet/message-broker-service/internal/models"
)

// Broker defines the interface for message broker operations
type Broker interface {
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	Publish(ctx context.Context, message *models.Message) error
	Subscribe(ctx context.Context, topic string, handler func(*models.Message) error) error
	IsHealthy() bool
	GetStats() map[string]interface{}
}

// Manager manages the message broker connection and operations
type Manager struct {
	broker Broker
	log    *logger.Logger
	config *config.BrokerConfig
}

// NewManager creates a new broker manager based on configuration
func NewManager(cfg *config.BrokerConfig, log *logger.Logger) (*Manager, error) {
	var broker Broker
	var err error

	switch cfg.Type {
	case "rabbitmq":
		broker, err = adapters.NewRabbitMQAdapter(&cfg.RabbitMQ, log)
	case "kafka":
		broker, err = adapters.NewKafkaAdapter(&cfg.Kafka, log)
	case "azure-servicebus":
		broker, err = adapters.NewAzureServiceBusAdapter(&cfg.AzureServiceBus, log)
	default:
		return nil, fmt.Errorf("unsupported broker type: %s", cfg.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create broker adapter: %w", err)
	}

	return &Manager{
		broker: broker,
		log:    log,
		config: cfg,
	}, nil
}

// Connect establishes connection to the message broker
func (m *Manager) Connect(ctx context.Context) error {
	m.log.Info("Connecting to message broker", "type", m.config.Type)
	
	if err := m.broker.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to broker: %w", err)
	}
	
	m.log.Info("Successfully connected to message broker", "type", m.config.Type)
	return nil
}

// Disconnect closes the connection to the message broker
func (m *Manager) Disconnect(ctx context.Context) error {
	m.log.Info("Disconnecting from message broker")
	
	if err := m.broker.Disconnect(ctx); err != nil {
		return fmt.Errorf("failed to disconnect from broker: %w", err)
	}
	
	m.log.Info("Successfully disconnected from message broker")
	return nil
}

// Publish publishes a message to the broker with tracing support
func (m *Manager) Publish(ctx context.Context, message *models.Message) error {
	// Create a span for broker publishing operation
	tracer := otel.Tracer("message-broker-service")
	ctx, span := tracer.Start(ctx, "broker.publish",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", m.config.Type),
			attribute.String("messaging.destination", message.Topic),
			attribute.String("message.id", message.ID),
			attribute.String("message.correlation_id", message.CorrelationID),
			attribute.String("message.source", message.Metadata.Source),
		),
	)
	defer span.End()

	// Log the publish operation
	m.log.Info("Publishing message to broker",
		"broker_type", m.config.Type,
		"topic", message.Topic,
		"message_id", message.ID,
		"correlation_id", message.CorrelationID,
	)

	// Publish using the underlying broker
	if err := m.broker.Publish(ctx, message); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, fmt.Sprintf("failed to publish message: %v", err))
		m.log.Error("Failed to publish message",
			"error", err,
			"broker_type", m.config.Type,
			"topic", message.Topic,
			"message_id", message.ID,
		)
		return fmt.Errorf("failed to publish message: %w", err)
	}

	span.SetStatus(codes.Ok, "message published successfully")
	m.log.Info("Message published successfully",
		"broker_type", m.config.Type,
		"topic", message.Topic,
		"message_id", message.ID,
	)

	return nil
}

// Subscribe subscribes to messages from a topic
func (m *Manager) Subscribe(ctx context.Context, topic string, handler func(*models.Message) error) error {
	return m.broker.Subscribe(ctx, topic, handler)
}

// IsHealthy checks if the broker connection is healthy
func (m *Manager) IsHealthy() bool {
	return m.broker.IsHealthy()
}

// GetStats returns broker statistics
func (m *Manager) GetStats() map[string]interface{} {
	stats := m.broker.GetStats()
	stats["broker_type"] = m.config.Type
	return stats
}
