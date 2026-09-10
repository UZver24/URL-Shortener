package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// ClickHandler — функция-обработчик событий клика
type ClickHandler func(ctx context.Context, event *ClickEvent) error

// Consumer потребляет события из Kafka
type Consumer struct {
	reader  *kafkago.Reader
	handler ClickHandler
	logger  *slog.Logger
	wg      sync.WaitGroup
	quit    chan struct{}
}

// ConsumerConfig — конфигурация Consumer
type ConsumerConfig struct {
	Brokers     []string     // Адреса брокеров Kafka
	Topic       string       // Имя топика
	GroupID     string       // Consumer Group ID
	Handler     ClickHandler // Обработчик событий
	Logger      *slog.Logger // Логгер
	MinBytes    int          // Минимальный размер батча (по умолчанию 10KB)
	MaxBytes    int          // Максимальный размер батча (по умолчанию 10MB)
	StartOffset int64        // Начальный offset (kafkago.FirstOffset или kafkago.LastOffset)
}

// NewConsumer создаёт новый Kafka Consumer
func NewConsumer(cfg ConsumerConfig) (*Consumer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers list is empty")
	}

	if cfg.Topic == "" {
		return nil, fmt.Errorf("kafka topic is empty")
	}

	if cfg.GroupID == "" {
		return nil, fmt.Errorf("consumer group id is empty")
	}

	if cfg.Handler == nil {
		return nil, fmt.Errorf("click handler is nil")
	}

	// Настройки батчинга
	minBytes := cfg.MinBytes
	if minBytes <= 0 {
		minBytes = 10e3 // 10KB
	}

	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 10e6 // 10MB
	}

	startOffset := cfg.StartOffset
	if startOffset == 0 {
		startOffset = kafkago.LastOffset // Читаем только новые сообщения
	}

	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        cfg.Brokers,
		Topic:          cfg.Topic,
		GroupID:        cfg.GroupID,
		MinBytes:       minBytes,
		MaxBytes:       maxBytes,
		StartOffset:    startOffset,
		CommitInterval: time.Second, // Коммитим offset каждую секунду
		MaxWait:        500 * time.Millisecond,
	})

	return &Consumer{
		reader:  reader,
		handler: cfg.Handler,
		logger:  cfg.Logger,
		quit:    make(chan struct{}),
	}, nil
}

// Start запускает потребление сообщений в отдельной горутине
func (c *Consumer) Start() {
	c.wg.Add(1)
	go c.consume()
	c.logger.Info("kafka consumer started",
		"group_id", c.reader.Config().GroupID,
		"topic", c.reader.Config().Topic,
	)
}

// consume — основной цикл потребления сообщений
func (c *Consumer) consume() {
	defer c.wg.Done()

	for {
		select {
		case <-c.quit:
			c.logger.Info("kafka consumer stopping")
			return
		default:
			c.processMessage()
		}
	}
}

// processMessage читает и обрабатывает одно сообщение
func (c *Consumer) processMessage() {
	// Создаём контекст с таймаутом для чтения
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Читаем сообщение
	msg, err := c.reader.FetchMessage(ctx)
	if err != nil {
		// Проверяем, не закрыт ли consumer
		select {
		case <-c.quit:
			return // Штатное завершение
		default:
		}

		// Логируем ошибку и продолжаем (Kafka может быть временно недоступен)
		if ctx.Err() == nil { // Не таймаут
			c.logger.Warn("failed to fetch message", "error", err)
		}
		return
	}

	// Десериализуем событие
	event, err := DeserializeClickEvent(msg.Value)
	if err != nil {
		c.logger.Error("failed to deserialize click event",
			"offset", msg.Offset,
			"partition", msg.Partition,
			"error", err,
		)
		// Коммитим offset, чтобы не зависнуть на битом сообщении
		_ = c.reader.CommitMessages(ctx, msg)
		return
	}

	// Обрабатываем событие с retry
	c.handleWithRetry(ctx, event, msg)
}

// handleWithRetry обрабатывает событие с exponential backoff
func (c *Consumer) handleWithRetry(ctx context.Context, event *ClickEvent, msg kafkago.Message) {
	const maxRetries = 5
	baseDelay := 1 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Создаём контекст с таймаутом для обработки
		handlerCtx, cancel := context.WithTimeout(ctx, 10*time.Second)

		err := c.handler(handlerCtx, event)
		cancel()

		if err == nil {
			// Успех — коммитим offset
			commitCtx, commitCancel := context.WithTimeout(ctx, 2*time.Second)
			if err := c.reader.CommitMessages(commitCtx, msg); err != nil {
				c.logger.Error("failed to commit message",
					"offset", msg.Offset,
					"partition", msg.Partition,
					"error", err,
				)
			}
			commitCancel()

			c.logger.Debug("click event processed",
				"link_id", event.LinkID,
				"short_code", event.ShortCode,
				"attempt", attempt+1,
			)
			return
		}

		c.logger.Warn("failed to handle click event, retrying",
			"link_id", event.LinkID,
			"short_code", event.ShortCode,
			"attempt", attempt+1,
			"error", err,
		)

		// Exponential backoff (кроме последней попытки)
		if attempt < maxRetries-1 {
			delay := baseDelay * time.Duration(1<<uint(attempt))
			if delay > 16*time.Second {
				delay = 16 * time.Second
			}

			select {
			case <-time.After(delay):
				// Продолжаем retry
			case <-c.quit:
				return // Штатное завершение
			}
		}
	}

	// Все попытки провалились — логируем и коммитим (чтобы не зависнуть)
	c.logger.Error("failed to handle click event after all retries",
		"link_id", event.LinkID,
		"short_code", event.ShortCode,
		"retries", maxRetries,
	)

	commitCtx, commitCancel := context.WithTimeout(ctx, 2*time.Second)
	defer commitCancel()
	_ = c.reader.CommitMessages(commitCtx, msg)
}

// Stop останавливает consumer
func (c *Consumer) Stop() error {
	close(c.quit)
	c.wg.Wait()

	if err := c.reader.Close(); err != nil {
		return fmt.Errorf("close kafka reader: %w", err)
	}

	c.logger.Info("kafka consumer stopped")
	return nil
}
