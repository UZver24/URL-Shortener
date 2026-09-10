package redis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// AnalyticsCache реализует оптимизированную аналитику через Redis:
// - HyperLogLog для уникальных переходов (O(1) добавление, O(1) получение)
// - Sorted Set для топа популярных ссылок (O(log n) обновление, O(k + log n) получение)
// - Sliding Window для "горячих" ссылок (trending)
type AnalyticsCache struct {
	client *redis.Client
	logger *slog.Logger
}

// NewAnalyticsCache создаёт новый кэш для аналитики
func NewAnalyticsCache(client *redis.Client, logger *slog.Logger) *AnalyticsCache {
	return &AnalyticsCache{
		client: client,
		logger: logger,
	}
}

// ==================== HyperLogLog (уникальные переходы) ====================

// RecordUniqueClick записывает уникальный переход через HyperLogLog
// Сложность: O(1)
// Память: ~12KB на ключ (фиксированная, не зависит от количества уникальных visitors)
// Точность: ~0.81% стандартная ошибка
func (c *AnalyticsCache) RecordUniqueClick(ctx context.Context, linkID int64, visitorID string) error {
	key := fmt.Sprintf("link:unique:%d", linkID)
	if err := c.client.PFAdd(ctx, key, visitorID).Err(); err != nil {
		return fmt.Errorf("pfadd unique click: %w", err)
	}
	return nil
}

// GetUniqueClicks возвращает количество уникальных переходов
// Сложность: O(1)
func (c *AnalyticsCache) GetUniqueClicks(ctx context.Context, linkID int64) (int64, error) {
	key := fmt.Sprintf("link:unique:%d", linkID)
	count, err := c.client.PFCount(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("pfcount unique clicks: %w", err)
	}
	return count, nil
}

// ==================== Sorted Set (топ популярных ссылок) ====================

// IncrementPopularity увеличивает счётчик популярности ссылки
// Сложность: O(log N) где N — количество элементов в sorted set
func (c *AnalyticsCache) IncrementPopularity(ctx context.Context, linkID int64, shortCode string) error {
	key := "link:popularity"
	// Используем short_code как member (более читаемо чем link_id)
	if err := c.client.ZIncrBy(ctx, key, 1, shortCode).Err(); err != nil {
		return fmt.Errorf("zincrby popularity: %w", err)
	}
	return nil
}

// GetTopLinks возвращает топ N популярных ссылок
// Сложность: O(log(N) + M) где M — количество запрашиваемых элементов
func (c *AnalyticsCache) GetTopLinks(ctx context.Context, limit int) ([]TopLink, error) {
	key := "link:popularity"
	results, err := c.client.ZRevRangeWithScores(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("zrevrange top links: %w", err)
	}

	links := make([]TopLink, 0, len(results))
	for _, z := range results {
		links = append(links, TopLink{
			ShortCode: z.Member.(string),
			Clicks:    int64(z.Score),
		})
	}
	return links, nil
}

// ==================== Sliding Window (trending / "горячие" ссылки) ====================

// RecordTrendingClick записывает клик для определения "горячих" ссылок
// Использует sliding window с гранулярностью 1 минута, окно 60 минут
// Сложность: O(1)
func (c *AnalyticsCache) RecordTrendingClick(ctx context.Context, linkID int64, shortCode string) error {
	// Ключ включает текущую минуту для гранулярности
	minute := time.Now().Unix() / 60
	key := fmt.Sprintf("link:trending:%d", minute)

	pipe := c.client.Pipeline()
	pipe.ZIncrBy(ctx, key, 1, shortCode)
	pipe.Expire(ctx, key, 65*time.Minute) // TTL чуть больше окна (65 > 60)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("record trending click: %w", err)
	}
	return nil
}

// GetTrendingLinks возвращает "горячие" ссылки за последний час
// Суммирует клики за последние 60 минут
// Сложность: O(M * log(N)) где M = 60 (минуты), N = количество ссылок
func (c *AnalyticsCache) GetTrendingLinks(ctx context.Context, limit int) ([]TopLink, error) {
	now := time.Now().Unix() / 60

	// Собираем ключи за последние 60 минут
	keys := make([]string, 0, 60)
	for i := 0; i < 60; i++ {
		minute := now - int64(i)
		keys = append(keys, fmt.Sprintf("link:trending:%d", minute))
	}

	// Используем временный ключ для объединения (ZUNIONSTORE)
	tmpKey := fmt.Sprintf("link:trending:tmp:%d", time.Now().UnixNano())
	defer c.client.Del(ctx, tmpKey) // cleanup

	// ZUNIONSTORE объединяет sorted sets
	if err := c.client.ZUnionStore(ctx, tmpKey, &redis.ZStore{
		Keys:      keys,
		Aggregate: "sum",
	}).Err(); err != nil {
		return nil, fmt.Errorf("zunionstore trending: %w", err)
	}

	// Получаем топ из объединённого результата
	results, err := c.client.ZRevRangeWithScores(ctx, tmpKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("zrevrange trending: %w", err)
	}

	links := make([]TopLink, 0, len(results))
	for _, z := range results {
		links = append(links, TopLink{
			ShortCode: z.Member.(string),
			Clicks:    int64(z.Score),
		})
	}
	return links, nil
}

// TopLink — элемент топа ссылок (short_code + количество кликов)
type TopLink struct {
	ShortCode string `json:"short_code"`
	Clicks    int64  `json:"clicks"`
}
