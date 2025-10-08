package adapters

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/aioutlet/message-broker-service/internal/config"
	"github.com/aioutlet/message-broker-service/internal/logger"
	"github.com/aioutlet/message-broker-service/internal/models"
)

// KafkaAdapter implements the Broker interface for Kafka
type KafkaAdapter struct {
	config    *config.KafkaConfig
	log       *logger.Logger
	writer    *kafka.Writer
	reader    *kafka.Reader
	connected bool
	mu        sync.RWMutex
	stats     Stats
}

// NewKafkaAdapter creates a new Kafka adapter
func NewKafkaAdapter(cfg *config.KafkaConfig, log *logger.Logger) (*KafkaAdapter, error) {
	return &KafkaAdapter{
		config: cfg,
		log:    log.WithFields(map[string]interface{}{"adapter": "kafka"}),
	}, nil
}

// Connect establishes connection to Kafka
func (k *KafkaAdapter) Connect(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.log.Info("Connecting to Kafka", "brokers", k.config.Brokers)

	// Create writer for publishing
	k.writer = &kafka.Writer{
		Addr:         kafka.TCP(k.config.Brokers...),
		Topic:        k.config.Topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireAll,
		MaxAttempts:  3,
		Async:        false, // Synchronous writes for reliability
		Compression:  kafka.Snappy,
		Logger:       kafka.LoggerFunc(k.log.Infof),
		ErrorLogger:  kafka.LoggerFunc(k.log.Errorf),
	}

	// Test connection by fetching metadata
	conn, err := kafka.DialLeader(ctx, "tcp", k.config.Brokers[0], k.config.Topic, 0)
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka: %w", err)
	}
	conn.Close()

	k.connected = true

	k.log.Info("Successfully connected to Kafka",
		"brokers", k.config.Brokers,
		"topic", k.config.Topic,
	)

	return nil
}

// Disconnect closes the Kafka connections
func (k *KafkaAdapter) Disconnect(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if !k.connected {
		return nil
	}

	k.log.Info("Disconnecting from Kafka")

	if k.writer != nil {
		if err := k.writer.Close(); err != nil {
			k.log.Warn("Error closing writer", "error", err)
		}
	}

	if k.reader != nil {
		if err := k.reader.Close(); err != nil {
			k.log.Warn("Error closing reader", "error", err)
		}
	}

	k.connected = false
	return nil
}

// Publish publishes a message to Kafka
func (k *KafkaAdapter) Publish(ctx context.Context, message *models.Message) error {
	k.mu.RLock()
	if !k.connected || k.writer == nil {
		k.mu.RUnlock()
		return models.ErrBrokerNotConnected
	}
	writer := k.writer
	k.mu.RUnlock()

	// Validate message
	if err := message.Validate(); err != nil {
		k.updateStats(false)
		return err
	}

	// Convert message to JSON
	body, err := message.ToJSON()
	if err != nil {
		k.updateStats(false)
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Prepare headers
	headers := []kafka.Header{
		{Key: "id", Value: []byte(message.ID)},
		{Key: "source", Value: []byte(message.Metadata.Source)},
		{Key: "content-type", Value: []byte(message.Metadata.ContentType)},
	}

	if message.CorrelationID != "" {
		headers = append(headers, kafka.Header{
			Key:   "correlation-id",
			Value: []byte(message.CorrelationID),
		})
	}

	// Add custom headers
	for k, v := range message.Metadata.Headers {
		headers = append(headers, kafka.Header{
			Key:   k,
			Value: []byte(v),
		})
	}

	// Create Kafka message
	kafkaMsg := kafka.Message{
		Key:     []byte(message.Topic), // Use topic as key for partitioning
		Value:   body,
		Headers: headers,
		Time:    message.Timestamp,
	}

	// Publish message
	err = writer.WriteMessages(ctx, kafkaMsg)
	if err != nil {
		k.updateStats(false)
		return fmt.Errorf("failed to publish message: %w", err)
	}

	k.updateStats(true)
	k.log.Debug("Message published",
		"messageId", message.ID,
		"topic", message.Topic,
	)

	return nil
}

// Subscribe subscribes to messages from a topic
func (k *KafkaAdapter) Subscribe(ctx context.Context, topic string, handler func(*models.Message) error) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if !k.connected {
		return models.ErrBrokerNotConnected
	}

	// Create reader for consuming
	k.reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        k.config.Brokers,
		Topic:          k.config.Topic,
		GroupID:        k.config.GroupID,
		MinBytes:       10e3, // 10KB
		MaxBytes:       10e6, // 10MB
		CommitInterval: time.Second,
		StartOffset:    kafka.LastOffset,
		Logger:         kafka.LoggerFunc(k.log.Infof),
		ErrorLogger:    kafka.LoggerFunc(k.log.Errorf),
	})

	// Start consuming in a goroutine
	go func() {
		for {
			msg, err := k.reader.ReadMessage(ctx)
			if err != nil {
				if err == context.Canceled {
					return
				}
				k.log.Error("Failed to read message", "error", err)
				continue
			}

			// Parse message
			message, err := models.FromJSON(msg.Value)
			if err != nil {
				k.log.Error("Failed to parse message", "error", err)
				continue
			}

			// Call handler
			if err := handler(message); err != nil {
				k.log.Error("Handler error", "error", err, "messageId", message.ID)
				// Note: Message will be automatically retried by Kafka
				continue
			}

			// Commit is automatic with CommitInterval
		}
	}()

	k.log.Info("Subscribed to topic", "topic", topic, "groupId", k.config.GroupID)
	return nil
}

// IsHealthy checks if the connection is healthy
func (k *KafkaAdapter) IsHealthy() bool {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.connected && k.writer != nil
}

// GetStats returns adapter statistics
func (k *KafkaAdapter) GetStats() map[string]interface{} {
	k.stats.mu.RLock()
	defer k.stats.mu.RUnlock()

	stats := map[string]interface{}{
		"messages_published": k.stats.MessagesPublished,
		"messages_failed":    k.stats.MessagesFailed,
		"last_published_at":  k.stats.LastPublishedAt,
		"connected":          k.IsHealthy(),
	}

	// Add writer stats if available
	if k.writer != nil {
		writerStats := k.writer.Stats()
		stats["writer_writes"] = writerStats.Writes
		stats["writer_messages"] = writerStats.Messages
		stats["writer_errors"] = writerStats.Errors
	}

	return stats
}

// updateStats updates adapter statistics
func (k *KafkaAdapter) updateStats(success bool) {
	k.stats.mu.Lock()
	defer k.stats.mu.Unlock()

	if success {
		k.stats.MessagesPublished++
		k.stats.LastPublishedAt = time.Now()
	} else {
		k.stats.MessagesFailed++
	}
}
