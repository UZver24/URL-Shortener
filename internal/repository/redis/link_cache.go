package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/UZver24/URL-Shortener/internal/model"
	"github.com/redis/go-redis/v9"
)

// LinkCache реализует кэш-слой для ссылок
type LinkCache struct {
	client *redis.Client
	logger *slog.Logger
	ttl    time.Duration
}

// NewLinkCache создаёт новый кэш для ссылок
// ttl: время жизни записи в кэше
func NewLinkCache(client *redis.Client, logger *slog.Logger, ttl time.Duration) *LinkCache {
	return &LinkCache{
		client: client,
		logger: logger,
		ttl:    ttl,
	}
}

// cacheKey генерирует ключ для Redis
func cacheKey(shortCode string) string {
	return fmt.Sprintf("link:%s", shortCode)
}

// Get возвращает ссылку из кэша
// Если ссылки нет — возвращает model.ErrLinkNotFound
func (c *LinkCache) Get(ctx context.Context, shortCode string) (*model.Link, error) {
	key := cacheKey(shortCode)

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Ключ не найден — это нормальная ситуация (cache miss)
			return nil, model.ErrLinkNotFound
		}
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var link model.Link
	if err := json.Unmarshal(data, &link); err != nil {
		return nil, fmt.Errorf("unmarshal link: %w", err)
	}

	return &link, nil
}

// Set сохраняет ссылку в кэш с TTL
func (c *LinkCache) Set(ctx context.Context, link *model.Link) error {
	if link == nil {
		return nil
	}

	key := cacheKey(link.ShortCode)

	data, err := json.Marshal(link)
	if err != nil {
		return fmt.Errorf("marshal link: %w", err)
	}

	if err := c.client.Set(ctx, key, data, c.ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}

	c.logger.Debug("link cached", "short_code", link.ShortCode, "ttl", c.ttl)
	return nil
}

// Delete удаляет ссылку из кэша (инвалидация)
func (c *LinkCache) Delete(ctx context.Context, shortCode string) error {
	key := cacheKey(shortCode)

	if err := c.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}

	c.logger.Debug("link invalidated in cache", "short_code", shortCode)
	return nil
}
