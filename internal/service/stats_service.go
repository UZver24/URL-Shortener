package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/UZver24/URL-Shortener/internal/messaging/kafka"
	"github.com/UZver24/URL-Shortener/internal/repository/postgres/stats"
)

// StatsService реализует бизнес-логику для статистики
type StatsService struct {
	repo   *stats.StatsRepository
	logger *slog.Logger
}

// NewStatsService создаёт новый сервис статистики
func NewStatsService(repo *stats.StatsRepository, logger *slog.Logger) *StatsService {
	return &StatsService{
		repo:   repo,
		logger: logger,
	}
}

// HandleClickEvent обрабатывает событие клика из Kafka
func (s *StatsService) HandleClickEvent(ctx context.Context, event *kafka.ClickEvent) error {
	// Конвертируем kafka.ClickEvent в stats.ClickEvent
	statsEvent := &stats.ClickEvent{
		LinkID:    event.LinkID,
		ShortCode: event.ShortCode,
		UserAgent: event.UserAgent,
		Referer:   event.Referer,
		Timestamp: event.Timestamp,
	}

	// Записываем в БД
	if err := s.repo.RecordClick(ctx, statsEvent); err != nil {
		s.logger.Error("failed to record click event",
			"link_id", event.LinkID,
			"short_code", event.ShortCode,
			"error", err,
		)
		return fmt.Errorf("record click event: %w", err)
	}

	s.logger.Debug("click event recorded",
		"link_id", event.LinkID,
		"short_code", event.ShortCode,
	)

	return nil
}

// GetStats возвращает статистику по ссылке
func (s *StatsService) GetStats(ctx context.Context, shortCode string) (*stats.LinkStats, error) {
	statsData, err := s.repo.GetStats(ctx, shortCode)
	if err != nil {
		s.logger.Warn("failed to get stats", "short_code", shortCode, "error", err)
		return nil, err
	}

	return statsData, nil
}

// GetTopLinks возвращает топ популярных ссылок
func (s *StatsService) GetTopLinks(ctx context.Context, limit int) ([]stats.TopLink, error) {
	links, err := s.repo.GetTopLinks(ctx, limit)
	if err != nil {
		s.logger.Error("failed to get top links", "error", err)
		return nil, err
	}

	return links, nil
}

// GetTrending возвращает "горячие" ссылки
func (s *StatsService) GetTrending(ctx context.Context, limit int) ([]stats.TopLink, error) {
	links, err := s.repo.GetTrending(ctx, limit)
	if err != nil {
		s.logger.Error("failed to get trending links", "error", err)
		return nil, err
	}

	return links, nil
}
