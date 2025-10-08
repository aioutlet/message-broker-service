package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// MessageBrokerClient is a simple HTTP client for the message broker service
type MessageBrokerClient struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

// PublishRequest represents a message to be published
type PublishRequest struct {
	Topic         string                 `json:"topic"`
	Data          map[string]interface{} `json:"data"`
	CorrelationID string                 `json:"correlationId,omitempty"`
	Priority      int                    `json:"priority,omitempty"`
}

// PublishResponse represents the response from publishing
type PublishResponse struct {
	Success   bool      `json:"success"`
	MessageID string    `json:"messageId"`
	Topic     string    `json:"topic"`
	Timestamp time.Time `json:"timestamp"`
}

// NewClient creates a new message broker client
func NewClient(baseURL, apiKey string) *MessageBrokerClient {
	return &MessageBrokerClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Publish publishes a message to the broker
func (c *MessageBrokerClient) Publish(topic string, data map[string]interface{}) (*PublishResponse, error) {
	// Prepare request
	reqBody := PublishRequest{
		Topic: topic,
		Data:  data,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", c.BaseURL+"/api/v1/publish", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	// Send request
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var publishResp PublishResponse
	if err := json.NewDecoder(resp.Body).Decode(&publishResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	return &publishResp, nil
}

// Example usage
func main() {
	// Create client
	client := NewClient("http://localhost:4000", "your-api-key-here")

	// Example 1: Publish auth.login event
	fmt.Println("📤 Publishing auth.login event...")
	resp, err := client.Publish("auth.login", map[string]interface{}{
		"userId":    "123",
		"email":     "user@example.com",
		"ipAddress": "192.168.1.1",
		"timestamp": time.Now().Unix(),
	})

	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		return
	}

	fmt.Printf("✅ Success! Message ID: %s\n", resp.MessageID)
	fmt.Printf("   Topic: %s\n", resp.Topic)
	fmt.Printf("   Timestamp: %s\n", resp.Timestamp)

	// Example 2: Publish order.placed event
	fmt.Println("\n📤 Publishing order.placed event...")
	resp2, err := client.Publish("order.placed", map[string]interface{}{
		"orderId":     "ORD-12345",
		"userId":      "123",
		"totalAmount": 99.99,
		"items": []map[string]interface{}{
			{
				"productId": "PROD-001",
				"quantity":  2,
				"price":     49.99,
			},
		},
	})

	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		return
	}

	fmt.Printf("✅ Success! Message ID: %s\n", resp2.MessageID)

	// Example 3: Check health
	fmt.Println("\n🏥 Checking service health...")
	healthResp, err := http.Get("http://localhost:4000/api/v1/health")
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		return
	}
	defer healthResp.Body.Close()

	var health map[string]interface{}
	json.NewDecoder(healthResp.Body).Decode(&health)
	fmt.Printf("✅ Service status: %v\n", health["status"])
}
