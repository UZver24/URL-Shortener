package kafka

import (
	"testing"
	"time"
)

func TestClickEvent_Serialize(t *testing.T) {
	event := &ClickEvent{
		LinkID:    123,
		ShortCode: "abc123",
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		UserAgent: "Mozilla/5.0",
		Referer:   "https://example.com",
	}

	data, err := event.Serialize()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("serialized data is empty")
	}

	// Десериализуем и проверяем
	deserialized, err := DeserializeClickEvent(data)
	if err != nil {
		t.Fatalf("failed to deserialize: %v", err)
	}

	if deserialized.LinkID != event.LinkID {
		t.Errorf("expected LinkID %d, got %d", event.LinkID, deserialized.LinkID)
	}

	if deserialized.ShortCode != event.ShortCode {
		t.Errorf("expected ShortCode %s, got %s", event.ShortCode, deserialized.ShortCode)
	}

	if deserialized.UserAgent != event.UserAgent {
		t.Errorf("expected UserAgent %s, got %s", event.UserAgent, deserialized.UserAgent)
	}

	if deserialized.Referer != event.Referer {
		t.Errorf("expected Referer %s, got %s", event.Referer, deserialized.Referer)
	}
}

func TestNewClickEvent(t *testing.T) {
	linkID := int64(456)
	shortCode := "xyz789"
	userAgent := "TestAgent"
	referer := "https://test.com"

	event := NewClickEvent(linkID, shortCode, userAgent, referer)

	if event.LinkID != linkID {
		t.Errorf("expected LinkID %d, got %d", linkID, event.LinkID)
	}

	if event.ShortCode != shortCode {
		t.Errorf("expected ShortCode %s, got %s", shortCode, event.ShortCode)
	}

	if event.UserAgent != userAgent {
		t.Errorf("expected UserAgent %s, got %s", userAgent, event.UserAgent)
	}

	if event.Referer != referer {
		t.Errorf("expected Referer %s, got %s", referer, event.Referer)
	}

	if event.Timestamp.IsZero() {
		t.Error("expected Timestamp to be set")
	}
}

func TestDeserializeClickEvent_InvalidJSON(t *testing.T) {
	invalidJSON := []byte(`{"invalid": json}`)

	_, err := DeserializeClickEvent(invalidJSON)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestClickEvent_Serialize_EmptyFields(t *testing.T) {
	event := &ClickEvent{
		LinkID:    1,
		ShortCode: "test",
		Timestamp: time.Now(),
		// UserAgent и Referer пустые
	}

	data, err := event.Serialize()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	deserialized, err := DeserializeClickEvent(data)
	if err != nil {
		t.Fatalf("failed to deserialize: %v", err)
	}

	if deserialized.LinkID != event.LinkID {
		t.Errorf("expected LinkID %d, got %d", event.LinkID, deserialized.LinkID)
	}

	if deserialized.UserAgent != "" {
		t.Errorf("expected empty UserAgent, got %s", deserialized.UserAgent)
	}

	if deserialized.Referer != "" {
		t.Errorf("expected empty Referer, got %s", deserialized.Referer)
	}
}
