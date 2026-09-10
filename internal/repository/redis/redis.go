package redis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client обёртка над redis.Client с логированием
type Client struct {
	client *redis.Client
	logger *slog.Logger
}

// NewClient создаёт подключение к Redis
// addr: формат "host:port"
// password: пустая строка если без пароля
// db: номер БД (обычно 0)
func NewClient(addr, password string, db int, logger *slog.Logger) (*Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Проверяем соединение
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	logger.Info("connected to Redis", "addr", addr, "db", db)

	return &Client{
		client: client,
		logger: logger,
	}, nil
}

// Close закрывает соединение с Redis
func (c *Client) Close() error {
	if err := c.client.Close(); err != nil {
		return fmt.Errorf("close redis client: %w", err)
	}
	c.logger.Info("redis connection closed")
	return nil
}

// Ping проверяет доступность Redis
func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// GetClient возвращает внутренний redis.Client (для LinkCache)
func (c *Client) GetClient() *redis.Client {
	return c.client
}
