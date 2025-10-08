package models

import (
	"testing"
	"time"
)

func TestNewMessage(t *testing.T) {
	topic := "test.topic"
	data := map[string]interface{}{
		"key": "value",
	}
	source := "test-service"

	msg := NewMessage(topic, data, source)

	if msg.ID == "" {
		t.Error("Expected message ID to be generated")
	}

	if msg.Topic != topic {
		t.Errorf("Expected topic %s, got %s", topic, msg.Topic)
	}

	if msg.Metadata.Source != source {
		t.Errorf("Expected source %s, got %s", source, msg.Metadata.Source)
	}

	if msg.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
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
				Topic: "test.topic",
				Data:  map[string]interface{}{"key": "value"},
			},
			wantErr: false,
		},
		{
			name: "missing topic",
			message: &Message{
				Topic: "",
				Data:  map[string]interface{}{"key": "value"},
			},
			wantErr: true,
		},
		{
			name: "missing data",
			message: &Message{
				Topic: "test.topic",
				Data:  nil,
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
		ID:        "test-id",
		Topic:     "test.topic",
		Data:      map[string]interface{}{"key": "value"},
		Timestamp: time.Now().UTC(),
		Metadata: MessageMetadata{
			Source:      "test-service",
			ContentType: "application/json",
		},
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

	if msg2.Topic != msg.Topic {
		t.Errorf("Expected topic %s, got %s", msg.Topic, msg2.Topic)
	}
}
