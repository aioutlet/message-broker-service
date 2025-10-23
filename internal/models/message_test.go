package models

import (
	"testing"
	"time"
)

func TestNewMessage(t *testing.T) {
	source := "test-service"
	eventType := "test.created"
	eventVersion := "1.0"
	eventId := "evt-123"
	timestamp := time.Now().Format(time.RFC3339)
	data := map[string]interface{}{
		"key": "value",
	}
	metadata := map[string]interface{}{
		"environment": "test",
	}

	msg := NewMessage(source, eventType, eventVersion, eventId, timestamp, data, metadata)

	if msg.ID == "" {
		t.Error("Expected message ID to be generated")
	}

	if msg.Source != source {
		t.Errorf("Expected source %s, got %s", source, msg.Source)
	}

	if msg.EventType != eventType {
		t.Errorf("Expected eventType %s, got %s", eventType, msg.EventType)
	}

	if msg.EventVersion != eventVersion {
		t.Errorf("Expected eventVersion %s, got %s", eventVersion, msg.EventVersion)
	}

	if msg.EventID != eventId {
		t.Errorf("Expected eventId %s, got %s", eventId, msg.EventID)
	}

	if msg.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}

	if msg.GetRoutingKey() != eventType {
		t.Errorf("Expected routing key %s, got %s", eventType, msg.GetRoutingKey())
	}
}

func TestMessageValidation(t *testing.T) {
	tests := []struct {
		name    string
		message *Message
		wantErr bool
	}{
		{
			name: "valid message",
			message: &Message{
				Source:    "test-service",
				EventType: "test.created",
				Data:      map[string]interface{}{"key": "value"},
			},
			wantErr: false,
		},
		{
			name: "missing eventType",
			message: &Message{
				Source:    "test-service",
				EventType: "",
				Data:      map[string]interface{}{"key": "value"},
			},
			wantErr: true,
		},
		{
			name: "missing source",
			message: &Message{
				Source:    "",
				EventType: "test.created",
				Data:      map[string]interface{}{"key": "value"},
			},
			wantErr: true,
		},
		{
			name: "missing data",
			message: &Message{
				Source:    "test-service",
				EventType: "test.created",
				Data:      nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.message.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMessageJSONSerialization(t *testing.T) {
	msg := &Message{
		ID:           "test-id",
		Source:       "test-service",
		EventType:    "test.created",
		EventVersion: "1.0",
		EventID:      "evt-123",
		Data:         map[string]interface{}{"key": "value"},
		Metadata:     map[string]interface{}{"environment": "test"},
		Timestamp:    time.Now().UTC(),
	}

	// Marshal to JSON
	jsonData, err := msg.ToJSON()
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	// Unmarshal from JSON
	msg2, err := FromJSON(jsonData)
	if err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if msg2.ID != msg.ID {
		t.Errorf("Expected ID %s, got %s", msg.ID, msg2.ID)
	}

	if msg2.EventType != msg.EventType {
		t.Errorf("Expected eventType %s, got %s", msg.EventType, msg2.EventType)
	}

	if msg2.Source != msg.Source {
		t.Errorf("Expected source %s, got %s", msg.Source, msg2.Source)
	}
}

func TestSetAndGetTraceHeaders(t *testing.T) {
	msg := &Message{
		Source:    "test-service",
		EventType: "test.created",
		Data:      map[string]interface{}{"key": "value"},
	}

	traceHeaders := map[string]string{
		"traceparent": "00-abc123-def456-01",
		"tracestate":  "test=value",
	}

	msg.SetTraceHeaders(traceHeaders)

	retrieved := msg.GetTraceHeaders()
	if len(retrieved) != 2 {
		t.Errorf("Expected 2 trace headers, got %d", len(retrieved))
	}

	if retrieved["traceparent"] != traceHeaders["traceparent"] {
		t.Errorf("Expected traceparent %s, got %s", traceHeaders["traceparent"], retrieved["traceparent"])
	}
}
