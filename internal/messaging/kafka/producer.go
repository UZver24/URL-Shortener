package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// Producer публикует события в Kafka
type Producer struct {
	writer *kafkago.Writer
	logger *slog.Logger
	topic  string
}

// ProducerConfig — конфигурация Producer
type ProducerConfig struct {
	Brokers []string     // Адреса брокеров Kafka
	Topic   string       // Имя топика
	Logger  *slog.Logger // Логгер
}

// NewProducer создаёт новый Kafka Producer
func NewProducer(cfg ProducerConfig) (*Producer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers list is empty")
	}

	if cfg.Topic == "" {
		return nil, fmt.Errorf("kafka topic is empty")
	}

	// Writer с балансировкой по ключу (link_id для partitioning)
	writer := &kafkago.Writer{
		Addr:                   kafkago.TCP(cfg.Brokers...),
		Topic:                  cfg.Topic,
		Balancer:               &kafkago.LeastBytes{},
		AllowAutoTopicCreation: true,
		// Настройки надёжности
		RequiredAcks: kafkago.RequireOne, // Ждём подтверждение от leader
		MaxAttempts:  3,                  // Повторы при ошибках
		// Настройки производительности
		BatchSize:    100, // Батчинг сообщений
		BatchTimeout: 10 * time.Millisecond,
	}

	return &Producer{
		writer: writer,
		logger: cfg.Logger,
		topic:  cfg.Topic,
	}, nil
}

// PublishClick публикует событие клика в Kafka
// Использует link_id как ключ для partitioning (все клики одной ссылки → одна партиция)
func (p *Producer) PublishClick(ctx context.Context, event *ClickEvent) error {
	data, err := event.Serialize()
	if err != nil {
		return fmt.Errorf("serialize click event: %w", err)
	}

	// Ключ — link_id для партиционирования
	key := fmt.Sprintf("%d", event.LinkID)

	msg := kafkago.Message{
		Key:   []byte(key),
		Value: data,
	}

	// Публикуем с контекстом (для graceful shutdown)
	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		p.logger.Error("failed to publish click event",
			"link_id", event.LinkID,
			"short_code", event.ShortCode,
			"error", err,
		)
		return fmt.Errorf("write message to kafka: %w", err)
	}

	p.logger.Debug("click event published",
		"link_id", event.LinkID,
		"short_code", event.ShortCode,
	)

	return nil
}

// Close закрывает соединение с Kafka
func (p *Producer) Close() error {
	if err := p.writer.Close(); err != nil {
		return fmt.Errorf("close kafka writer: %w", err)
	}
	p.logger.Info("kafka producer closed")
	return nil
}
