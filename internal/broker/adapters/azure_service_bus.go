package adapters

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"

	"github.com/aioutlet/message-broker-service/internal/config"
	"github.com/aioutlet/message-broker-service/internal/logger"
	"github.com/aioutlet/message-broker-service/internal/models"
)

// AzureServiceBusAdapter implements the Broker interface for Azure Service Bus
type AzureServiceBusAdapter struct {
	config    *config.AzureServiceBusConfig
	log       *logger.Logger
	client    *azservicebus.Client
	sender    *azservicebus.Sender
	receiver  *azservicebus.Receiver
	connected bool
	mu        sync.RWMutex
	stats     Stats
}

// NewAzureServiceBusAdapter creates a new Azure Service Bus adapter
func NewAzureServiceBusAdapter(cfg *config.AzureServiceBusConfig, log *logger.Logger) (*AzureServiceBusAdapter, error) {
	return &AzureServiceBusAdapter{
		config: cfg,
		log:    log.WithFields(map[string]interface{}{"adapter": "azure-servicebus"}),
	}, nil
}

// Connect establishes connection to Azure Service Bus
func (a *AzureServiceBusAdapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.log.Info("Connecting to Azure Service Bus")

	// Create client
	client, err := azservicebus.NewClientFromConnectionString(a.config.ConnectionString, nil)
	if err != nil {
		return fmt.Errorf("failed to create Azure Service Bus client: %w", err)
	}

	// Create sender for the topic
	sender, err := client.NewSender(a.config.Topic, nil)
	if err != nil {
		client.Close(ctx)
		return fmt.Errorf("failed to create sender: %w", err)
	}

	a.client = client
	a.sender = sender
	a.connected = true

	a.log.Info("Successfully connected to Azure Service Bus",
		"topic", a.config.Topic,
	)

	return nil
}

// Disconnect closes the Azure Service Bus connection
func (a *AzureServiceBusAdapter) Disconnect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.connected {
		return nil
	}

	a.log.Info("Disconnecting from Azure Service Bus")

	if a.sender != nil {
		if err := a.sender.Close(ctx); err != nil {
			a.log.Warn("Error closing sender", "error", err)
		}
	}

	if a.receiver != nil {
		if err := a.receiver.Close(ctx); err != nil {
			a.log.Warn("Error closing receiver", "error", err)
		}
	}

	if a.client != nil {
		if err := a.client.Close(ctx); err != nil {
			a.log.Warn("Error closing client", "error", err)
		}
	}

	a.connected = false
	return nil
}

// Publish publishes a message to Azure Service Bus
func (a *AzureServiceBusAdapter) Publish(ctx context.Context, message *models.Message) error {
	a.mu.RLock()
	if !a.connected || a.sender == nil {
		a.mu.RUnlock()
		return models.ErrBrokerNotConnected
	}
	sender := a.sender
	a.mu.RUnlock()

	// Validate message
	if err := message.Validate(); err != nil {
		a.updateStats(false)
		return err
	}

	// Convert message to JSON
	body, err := message.ToJSON()
	if err != nil {
		a.updateStats(false)
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Create Service Bus message
	sbMsg := &azservicebus.Message{
		Body:        body,
		ContentType: &message.Metadata.ContentType,
		MessageID:   &message.ID,
		Subject:     &message.Topic, // Use Subject for routing
	}

	// Add correlation ID if present
	if message.CorrelationID != "" {
		sbMsg.CorrelationID = &message.CorrelationID
	}

	// Add custom properties
	if sbMsg.ApplicationProperties == nil {
		sbMsg.ApplicationProperties = make(map[string]interface{})
	}
	sbMsg.ApplicationProperties["source"] = message.Metadata.Source
	for k, v := range message.Metadata.Headers {
		sbMsg.ApplicationProperties[k] = v
	}

	// Send message
	err = sender.SendMessage(ctx, sbMsg, nil)
	if err != nil {
		a.updateStats(false)
		return fmt.Errorf("failed to publish message: %w", err)
	}

	a.updateStats(true)
	a.log.Debug("Message published",
		"messageId", message.ID,
		"topic", message.Topic,
	)

	return nil
}

// Subscribe subscribes to messages from a topic
func (a *AzureServiceBusAdapter) Subscribe(ctx context.Context, topic string, handler func(*models.Message) error) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.connected || a.client == nil {
		return models.ErrBrokerNotConnected
	}

	// Create receiver for the subscription
	// Note: In Azure Service Bus, you need to create a subscription first
	subscriptionName := "message-broker-service" // Could be configurable
	receiver, err := a.client.NewReceiverForSubscription(a.config.Topic, subscriptionName, nil)
	if err != nil {
		return fmt.Errorf("failed to create receiver: %w", err)
	}

	a.receiver = receiver

	// Start consuming in a goroutine
	go func() {
		for {
			messages, err := receiver.ReceiveMessages(ctx, 10, nil)
			if err != nil {
				if err == context.Canceled {
					return
				}
				a.log.Error("Failed to receive messages", "error", err)
				time.Sleep(time.Second) // Back off on error
				continue
			}

			for _, msg := range messages {
				// Parse message
				message, err := models.FromJSON(msg.Body)
				if err != nil {
					a.log.Error("Failed to parse message", "error", err)
					receiver.AbandonMessage(ctx, msg, nil) // Don't complete
					continue
				}

				// Call handler
				if err := handler(message); err != nil {
					a.log.Error("Handler error", "error", err, "messageId", message.ID)
					receiver.AbandonMessage(ctx, msg, nil) // Requeue
					continue
				}

				// Complete message
				if err := receiver.CompleteMessage(ctx, msg, nil); err != nil {
					a.log.Error("Failed to complete message", "error", err)
				}
			}
		}
	}()

	a.log.Info("Subscribed to topic", "topic", topic, "subscription", subscriptionName)
	return nil
}

// IsHealthy checks if the connection is healthy
func (a *AzureServiceBusAdapter) IsHealthy() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connected && a.client != nil
}

// GetStats returns adapter statistics
func (a *AzureServiceBusAdapter) GetStats() map[string]interface{} {
	a.stats.mu.RLock()
	defer a.stats.mu.RUnlock()

	return map[string]interface{}{
		"messages_published": a.stats.MessagesPublished,
		"messages_failed":    a.stats.MessagesFailed,
		"last_published_at":  a.stats.LastPublishedAt,
		"connected":          a.IsHealthy(),
	}
}

// updateStats updates adapter statistics
func (a *AzureServiceBusAdapter) updateStats(success bool) {
	a.stats.mu.Lock()
	defer a.stats.mu.Unlock()

	if success {
		a.stats.MessagesPublished++
		a.stats.LastPublishedAt = time.Now()
	} else {
		a.stats.MessagesFailed++
	}
}
