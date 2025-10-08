package adapters

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

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

// Publish publishes a message to RabbitMQ
func (r *RabbitMQAdapter) Publish(ctx context.Context, message *models.Message) error {
	r.mu.RLock()
	if !r.connected || r.channel == nil {
		r.mu.RUnlock()
		return models.ErrBrokerNotConnected
	}
	channel := r.channel
	r.mu.RUnlock()

	// Validate message
	if err := message.Validate(); err != nil {
		r.updateStats(false)
		return err
	}

	// Convert message to JSON
	body, err := message.ToJSON()
	if err != nil {
		r.updateStats(false)
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Prepare publishing
	publishing := amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent, // Make message persistent
		Timestamp:    message.Timestamp,
		MessageId:    message.ID,
		Priority:     uint8(message.Metadata.Priority),
		Headers:      amqp.Table{},
	}

	// Add correlation ID if present
	if message.CorrelationID != "" {
		publishing.CorrelationId = message.CorrelationID
	}

	// Add custom headers
	for k, v := range message.Metadata.Headers {
		publishing.Headers[k] = v
	}

	// Publish with context
	err = channel.PublishWithContext(
		ctx,
		r.config.Exchange, // exchange
		message.Topic,     // routing key
		false,             // mandatory
		false,             // immediate
		publishing,
	)

	if err != nil {
		r.updateStats(false)
		return fmt.Errorf("failed to publish message: %w", err)
	}

	r.updateStats(true)
	r.log.Debug("Message published",
		"messageId", message.ID,
		"topic", message.Topic,
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
