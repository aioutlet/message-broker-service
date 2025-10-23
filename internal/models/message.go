package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Message represents an AWS EventBridge-style event message
type Message struct {
	// AWS EventBridge standard fields
	ID            string                 `json:"id"`            // Message broker generated ID
	Source        string                 `json:"source"`        // Service that generated the event
	EventType     string                 `json:"eventType"`     // Type of event (used as routing key)
	EventVersion  string                 `json:"eventVersion"`  // Schema version
	EventID       string                 `json:"eventId"`       // Event-specific unique identifier
	Timestamp     time.Time              `json:"timestamp"`     // When event occurred
	CorrelationID string                 `json:"correlationId,omitempty"` // Request tracing
	Data          map[string]interface{} `json:"data"`          // Business data
	Metadata      map[string]interface{} `json:"metadata"`      // Additional context
	
	// Message broker specific fields
	Priority      int               `json:"priority,omitempty"`      // Message priority
	TraceHeaders  map[string]string `json:"traceHeaders,omitempty"`  // OpenTelemetry headers
}

// NewMessage creates a new message from a publish request
func NewMessage(source, eventType, eventVersion, eventId, timestamp string, data, metadata map[string]interface{}) *Message {
	now := time.Now().UTC()
	
	// Parse timestamp if provided, otherwise use current time
	var ts time.Time
	if timestamp != "" {
		parsed, err := time.Parse(time.RFC3339, timestamp)
		if err == nil {
			ts = parsed
		} else {
			ts = now
		}
	} else {
		ts = now
	}
	
	return &Message{
		ID:           uuid.New().String(),
		Source:       source,
		EventType:    eventType,
		EventVersion: eventVersion,
		EventID:      eventId,
		Timestamp:    ts,
		Data:         data,
		Metadata:     metadata,
		TraceHeaders: make(map[string]string),
	}
}

// SetTraceHeaders sets OpenTelemetry trace context headers
func (m *Message) SetTraceHeaders(traceHeaders map[string]string) {
	if m.TraceHeaders == nil {
		m.TraceHeaders = make(map[string]string)
	}
	
	for key, value := range traceHeaders {
		m.TraceHeaders[key] = value
	}
}

// GetTraceHeaders returns OpenTelemetry trace context headers
func (m *Message) GetTraceHeaders() map[string]string {
	if m.TraceHeaders == nil {
		return make(map[string]string)
	}
	
	traceHeaders := make(map[string]string)
	for key, value := range m.TraceHeaders {
		// Extract OpenTelemetry headers
		if key == "traceparent" || key == "tracestate" || key == "baggage" {
			traceHeaders[key] = value
		}
	}
	return traceHeaders
}

// GetRoutingKey returns the routing key for message broker (eventType)
func (m *Message) GetRoutingKey() string {
	return m.EventType
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
	if m.EventType == "" {
		return ErrInvalidTopic
	}
	if m.Source == "" {
		return ErrEmptyData
	}
	if m.Data == nil {
		return ErrEmptyData
	}
	return nil
}

// PublishRequest represents an HTTP publish request (AWS EventBridge-style)
// This is just an alias for Message for HTTP binding
type PublishRequest = Message

// PublishResponse represents an HTTP publish response
type PublishResponse struct {
	Success   bool      `json:"success"`
	MessageID string    `json:"messageId"`
	EventType string    `json:"eventType"`
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
