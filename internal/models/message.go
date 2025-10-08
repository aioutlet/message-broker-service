package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Message represents a message to be published or consumed
type Message struct {
	ID            string                 `json:"id"`
	Topic         string                 `json:"topic"`
	Data          map[string]interface{} `json:"data"`
	Metadata      MessageMetadata        `json:"metadata"`
	Timestamp     time.Time              `json:"timestamp"`
	CorrelationID string                 `json:"correlationId,omitempty"`
}

// MessageMetadata contains metadata about the message
type MessageMetadata struct {
	Source      string            `json:"source"`
	ContentType string            `json:"contentType"`
	Priority    int               `json:"priority,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// NewMessage creates a new message with generated ID and timestamp
func NewMessage(topic string, data map[string]interface{}, source string) *Message {
	return &Message{
		ID:        uuid.New().String(),
		Topic:     topic,
		Data:      data,
		Timestamp: time.Now().UTC(),
		Metadata: MessageMetadata{
			Source:      source,
			ContentType: "application/json",
			Headers:     make(map[string]string),
		},
	}
}

// ToJSON converts the message to JSON bytes
func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// FromJSON creates a message from JSON bytes
func FromJSON(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// Validate validates the message
func (m *Message) Validate() error {
	if m.Topic == "" {
		return ErrInvalidTopic
	}
	if m.Data == nil {
		return ErrEmptyData
	}
	return nil
}

// PublishRequest represents an HTTP publish request
type PublishRequest struct {
	Topic         string                 `json:"topic"`
	Data          map[string]interface{} `json:"data"`
	CorrelationID string                 `json:"correlationId,omitempty"`
	Priority      int                    `json:"priority,omitempty"`
}

// PublishResponse represents an HTTP publish response
type PublishResponse struct {
	Success   bool      `json:"success"`
	MessageID string    `json:"messageId"`
	Topic     string    `json:"topic"`
	Timestamp time.Time `json:"timestamp"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
}

// HealthResponse represents a health check response
type HealthResponse struct {
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Broker    map[string]interface{} `json:"broker"`
}
