package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"

	"github.com/UZver24/URL-Shortener/internal/messaging/kafka"
	"github.com/UZver24/URL-Shortener/internal/repository/postgres/stats"
	redisrepo "github.com/UZver24/URL-Shortener/internal/repository/redis"
)

// AnalyticsCache — интерфейс для Redis-аналитики (HyperLogLog, Sorted Set, Sliding Window)
type AnalyticsCache interface {
	RecordUniqueClick(ctx context.Context, linkID int64, visitorID string) error
	GetUniqueClicks(ctx context.Context, linkID int64) (int64, error)
	IncrementPopularity(ctx context.Context, linkID int64, shortCode string) error
	GetTopLinks(ctx context.Context, limit int) ([]redisrepo.TopLink, error)
	RecordTrendingClick(ctx context.Context, linkID int64, shortCode string) error
	GetTrendingLinks(ctx context.Context, limit int) ([]redisrepo.TopLink, error)
}

// StatsService реализует бизнес-логику для статистики
type StatsService struct {
	repo      *stats.StatsRepository
	analytics AnalyticsCache // опциональный Redis-кэш для аналитики (может быть nil)
	logger    *slog.Logger
}

// NewStatsService создаёт новый сервис статистики
// analytics может быть nil — тогда работаем без Redis-оптимизаций
func NewStatsService(repo *stats.StatsRepository, logger *slog.Logger, analytics AnalyticsCache) *StatsService {
	return &StatsService{
		repo:      repo,
		analytics: analytics,
		logger:    logger,
	}
}

// HandleClickEvent обрабатывает событие клика из Kafka
// Записывает в PostgreSQL + обновляет Redis-аналитику (HyperLogLog, Sorted Set, Sliding Window)
func (s *StatsService) HandleClickEvent(ctx context.Context, event *kafka.ClickEvent) error {
	// 1. Записываем в PostgreSQL (основное хранилище)
	statsEvent := &stats.ClickEvent{
		LinkID:    event.LinkID,
		ShortCode: event.ShortCode,
		UserAgent: event.UserAgent,
		Referer:   event.Referer,
		Timestamp: event.Timestamp,
	}

	if err := s.repo.RecordClick(ctx, statsEvent); err != nil {
		s.logger.Error("failed to record click event",
			"link_id", event.LinkID,
			"short_code", event.ShortCode,
			"error", err,
		)
		return fmt.Errorf("record click event: %w", err)
	}

	// 2. Обновляем Redis-аналитику (опционально, graceful degradation)
	if s.analytics != nil {
		s.updateAnalytics(ctx, event)
	}

	s.logger.Debug("click event recorded",
		"link_id", event.LinkID,
		"short_code", event.ShortCode,
	)

	return nil
}

// updateAnalytics обновляет все Redis-индексы аналитики
func (s *StatsService) updateAnalytics(ctx context.Context, event *kafka.ClickEvent) {
	// Генерируем visitor_id из User-Agent (для HyperLogLog)
	visitorID := generateVisitorID(event.UserAgent, event.Referer)

	// HyperLogLog: уникальный visitor
	if err := s.analytics.RecordUniqueClick(ctx, event.LinkID, visitorID); err != nil {
		s.logger.Warn("failed to record unique click",
			"link_id", event.LinkID,
			"error", err,
		)
	}

	// Sorted Set: популярность
	if err := s.analytics.IncrementPopularity(ctx, event.LinkID, event.ShortCode); err != nil {
		s.logger.Warn("failed to increment popularity",
			"link_id", event.LinkID,
			"error", err,
		)
	}

	// Sliding Window: trending
	if err := s.analytics.RecordTrendingClick(ctx, event.LinkID, event.ShortCode); err != nil {
		s.logger.Warn("failed to record trending click",
			"link_id", event.LinkID,
			"error", err,
		)
	}
}

// GetStats возвращает статистику по ссылке
// Если Redis доступен — добавляет unique_clicks из HyperLogLog
func (s *StatsService) GetStats(ctx context.Context, shortCode string) (*stats.LinkStats, error) {
	statsData, err := s.repo.GetStats(ctx, shortCode)
	if err != nil {
		s.logger.Warn("failed to get stats", "short_code", shortCode, "error", err)
		return nil, err
	}

	// Если Redis доступен — получаем unique_clicks из HyperLogLog
	if s.analytics != nil {
		uniqueClicks, err := s.analytics.GetUniqueClicks(ctx, statsData.LinkID)
		if err != nil {
			s.logger.Warn("failed to get unique clicks",
				"link_id", statsData.LinkID,
				"error", err,
			)
		} else {
			statsData.UniqueClicks = uniqueClicks
		}
	}

	return statsData, nil
}

// GetTopLinks возвращает топ популярных ссылок
// Приоритет: Redis Sorted Set (O(k + log n)) → PostgreSQL (O(n log n))
func (s *StatsService) GetTopLinks(ctx context.Context, limit int) ([]stats.TopLink, error) {
	// Пробуем Redis (быстрее)
	if s.analytics != nil {
		redisLinks, err := s.analytics.GetTopLinks(ctx, limit)
		if err == nil && len(redisLinks) > 0 {
			// Конвертируем в stats.TopLink
			result := make([]stats.TopLink, 0, len(redisLinks))
			for _, link := range redisLinks {
				result = append(result, stats.TopLink{
					ShortCode:   link.ShortCode,
					OriginalURL: "", // Redis не хранит original_url, можно дополнить из БД
					TotalClicks: link.Clicks,
				})
			}
			return result, nil
		}
		s.logger.Warn("failed to get top links from Redis, falling back to PostgreSQL", "error", err)
	}

	// Fallback: PostgreSQL
	links, err := s.repo.GetTopLinks(ctx, limit)
	if err != nil {
		s.logger.Error("failed to get top links from PostgreSQL", "error", err)
		return nil, err
	}

	return links, nil
}

// GetTrending возвращает "горячие" ссылки (активность за последний час)
// Приоритет: Redis Sliding Window (O(M * log N)) → PostgreSQL (O(n))
func (s *StatsService) GetTrending(ctx context.Context, limit int) ([]stats.TopLink, error) {
	// Пробуем Redis (быстрее для real-time)
	if s.analytics != nil {
		redisLinks, err := s.analytics.GetTrendingLinks(ctx, limit)
		if err == nil && len(redisLinks) > 0 {
			result := make([]stats.TopLink, 0, len(redisLinks))
			for _, link := range redisLinks {
				result = append(result, stats.TopLink{
					ShortCode:   link.ShortCode,
					OriginalURL: "",
					TotalClicks: link.Clicks,
				})
			}
			return result, nil
		}
		s.logger.Warn("failed to get trending from Redis, falling back to PostgreSQL", "error", err)
	}

	// Fallback: PostgreSQL
	links, err := s.repo.GetTrending(ctx, limit)
	if err != nil {
		s.logger.Error("failed to get trending from PostgreSQL", "error", err)
		return nil, err
	}

	return links, nil
}

// generateVisitorID генерирует идентификатор посетителя из User-Agent и Referer
// Используется для HyperLogLog (уникальные посетители)
func generateVisitorID(userAgent, referer string) string {
	data := userAgent + "|" + referer
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:8]) // 16 символов hex
}
