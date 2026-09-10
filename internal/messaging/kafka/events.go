package kafka

import (
	"encoding/json"
	"time"
)

// ClickEvent — событие клика по короткой ссылке
// Публикуется Link Service при redirect, потребляется Stats Service
type ClickEvent struct {
	LinkID    int64     `json:"link_id"`              // ID ссылки в БД
	ShortCode string    `json:"short_code"`           // Короткий код (для логирования)
	Timestamp time.Time `json:"timestamp"`            // Время клика
	UserAgent string    `json:"user_agent,omitempty"` // User-Agent (опционально, для аналитики)
	Referer   string    `json:"referer,omitempty"`    // Referer (опционально)
}

// EventType — тип события (для расширения в будущем)
type EventType string

const (
	EventTypeClick EventType = "click"
)

// Event — обёртка для событий с типом (для расширения в будущем)
type Event struct {
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// NewClickEvent создаёт событие клика
func NewClickEvent(linkID int64, shortCode, userAgent, referer string) *ClickEvent {
	return &ClickEvent{
		LinkID:    linkID,
		ShortCode: shortCode,
		Timestamp: time.Now(),
		UserAgent: userAgent,
		Referer:   referer,
	}
}

// Serialize сериализует ClickEvent в JSON
func (e *ClickEvent) Serialize() ([]byte, error) {
	return json.Marshal(e)
}

// DeserializeClickEvent десериализует JSON в ClickEvent
func DeserializeClickEvent(data []byte) (*ClickEvent, error) {
	var event ClickEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return &event, nil
}
