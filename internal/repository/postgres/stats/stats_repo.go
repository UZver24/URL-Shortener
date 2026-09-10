package stats

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ClickEvent — событие клика для записи в БД
type ClickEvent struct {
	LinkID    int64
	ShortCode string
	UserAgent string
	Referer   string
	Timestamp time.Time
}

// LinkStats — агрегированная статистика по ссылке
type LinkStats struct {
	LinkID        int64
	ShortCode     string
	OriginalURL   string
	TotalClicks   int64
	UniqueClicks  int64
	LastClickedAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// TopLink — элемент топа популярных ссылок
type TopLink struct {
	ShortCode   string `json:"short_code"`
	OriginalURL string `json:"original_url"`
	TotalClicks int64  `json:"total_clicks"`
}

// StatsRepository работает со статистикой в PostgreSQL
type StatsRepository struct {
	pool *pgxpool.Pool
}

// NewStatsRepository создаёт новый репозиторий статистики
func NewStatsRepository(pool *pgxpool.Pool) *StatsRepository {
	return &StatsRepository{pool: pool}
}

// RecordClick записывает событие клика и обновляет агрегированную статистику
// Использует транзакцию для атомарности
func (r *StatsRepository) RecordClick(ctx context.Context, event *ClickEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Записываем сырое событие клика
	insertClickQuery := `
		INSERT INTO click_events (link_id, short_code, user_agent, referer, clicked_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err = tx.Exec(ctx, insertClickQuery,
		event.LinkID,
		event.ShortCode,
		event.UserAgent,
		event.Referer,
		event.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert click event: %w", err)
	}

	// 2. Обновляем агрегированную статистику (UPSERT)
	upsertStatsQuery := `
		INSERT INTO link_stats (link_id, short_code, original_url, total_clicks, last_clicked_at, updated_at)
		VALUES ($1, $2, (SELECT original_url FROM links WHERE id = $1), 1, $3, NOW())
		ON CONFLICT (link_id) 
		DO UPDATE SET 
			total_clicks = link_stats.total_clicks + 1,
			last_clicked_at = $3,
			updated_at = NOW()
	`
	_, err = tx.Exec(ctx, upsertStatsQuery, event.LinkID, event.ShortCode, event.Timestamp)
	if err != nil {
		return fmt.Errorf("upsert link stats: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// GetStats возвращает статистику по ссылке
func (r *StatsRepository) GetStats(ctx context.Context, shortCode string) (*LinkStats, error) {
	query := `
		SELECT link_id, short_code, original_url, total_clicks, unique_clicks, 
		       last_clicked_at, created_at, updated_at
		FROM link_stats
		WHERE short_code = $1
	`

	stats := &LinkStats{}
	err := r.pool.QueryRow(ctx, query, shortCode).Scan(
		&stats.LinkID,
		&stats.ShortCode,
		&stats.OriginalURL,
		&stats.TotalClicks,
		&stats.UniqueClicks,
		&stats.LastClickedAt,
		&stats.CreatedAt,
		&stats.UpdatedAt,
	)

	if err != nil {
		// Если статистики нет — пробуем получить из links (для обратной совместимости)
		return r.getStatsFromLinks(ctx, shortCode)
	}

	return stats, nil
}

// getStatsFromLinks — fallback: получаем статистику из таблицы links (для ссылок без кликов)
func (r *StatsRepository) getStatsFromLinks(ctx context.Context, shortCode string) (*LinkStats, error) {
	query := `
		SELECT id, short_code, original_url, clicks, last_accessed_at, created_at
		FROM links
		WHERE short_code = $1
	`

	stats := &LinkStats{}
	var lastAccessedAt *time.Time

	err := r.pool.QueryRow(ctx, query, shortCode).Scan(
		&stats.LinkID,
		&stats.ShortCode,
		&stats.OriginalURL,
		&stats.TotalClicks,
		&lastAccessedAt,
		&stats.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("link not found: %w", err)
	}

	stats.LastClickedAt = lastAccessedAt
	stats.UpdatedAt = stats.CreatedAt

	return stats, nil
}

// GetTopLinks возвращает топ популярных ссылок
func (r *StatsRepository) GetTopLinks(ctx context.Context, limit int) ([]TopLink, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	query := `
		SELECT short_code, original_url, total_clicks
		FROM link_stats
		ORDER BY total_clicks DESC
		LIMIT $1
	`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query top links: %w", err)
	}
	defer rows.Close()

	var links []TopLink
	for rows.Next() {
		var link TopLink
		if err := rows.Scan(&link.ShortCode, &link.OriginalURL, &link.TotalClicks); err != nil {
			return nil, fmt.Errorf("scan top link: %w", err)
		}
		links = append(links, link)
	}

	return links, nil
}

// GetTrending возвращает "горячие" ссылки (с активностью за последний час)
func (r *StatsRepository) GetTrending(ctx context.Context, limit int) ([]TopLink, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	// Считаем клики за последний час
	query := `
		SELECT short_code, 
		       (SELECT original_url FROM links WHERE id = link_id) as original_url,
		       COUNT(*) as total_clicks
		FROM click_events
		WHERE clicked_at > NOW() - INTERVAL '1 hour'
		GROUP BY link_id, short_code
		ORDER BY total_clicks DESC
		LIMIT $1
	`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query trending links: %w", err)
	}
	defer rows.Close()

	var links []TopLink
	for rows.Next() {
		var link TopLink
		if err := rows.Scan(&link.ShortCode, &link.OriginalURL, &link.TotalClicks); err != nil {
			return nil, fmt.Errorf("scan trending link: %w", err)
		}
		links = append(links, link)
	}

	return links, nil
}
