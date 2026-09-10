package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/UZver24/URL-Shortener/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LinkRepository реализует работу с таблицей links
type LinkRepository struct {
	pool *pgxpool.Pool
}

// NewLinkRepository создаёт новый репозиторий
func NewLinkRepository(pool *pgxpool.Pool) *LinkRepository {
	return &LinkRepository{pool: pool}
}

// Create создаёт новую ссылку в БД
func (r *LinkRepository) Create(ctx context.Context, link *model.Link) error {
	query := `
		INSERT INTO links (short_code, original_url, created_at, clicks, last_accessed_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	err := r.pool.QueryRow(
		ctx, query,
		link.ShortCode,
		link.OriginalURL,
		link.CreatedAt,
		link.Clicks,
		link.LastAccessedAt,
	).Scan(&link.ID)

	if err != nil {
		// Проверяем, является ли ошибка дубликатом short_code
		if isDuplicateError(err) {
			return model.ErrShortCodeAlreadyTaken
		}
		return fmt.Errorf("insert link: %w", err)
	}

	return nil
}

// GetByCode возвращает ссылку по короткому коду
func (r *LinkRepository) GetByCode(ctx context.Context, code string) (*model.Link, error) {
	query := `
		SELECT id, short_code, original_url, created_at, clicks, last_accessed_at
		FROM links
		WHERE short_code = $1
	`

	link := &model.Link{}
	err := r.pool.QueryRow(ctx, query, code).Scan(
		&link.ID,
		&link.ShortCode,
		&link.OriginalURL,
		&link.CreatedAt,
		&link.Clicks,
		&link.LastAccessedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrLinkNotFound
		}
		return nil, fmt.Errorf("select link: %w", err)
	}

	return link, nil
}

// GetByOriginalURL возвращает ссылку по оригинальному URL
func (r *LinkRepository) GetByOriginalURL(ctx context.Context, originalURL string) (*model.Link, error) {
	query := `
		SELECT id, short_code, original_url, created_at, clicks, last_accessed_at
		FROM links
		WHERE original_url = $1
		LIMIT 1
	`

	link := &model.Link{}
	err := r.pool.QueryRow(ctx, query, originalURL).Scan(
		&link.ID,
		&link.ShortCode,
		&link.OriginalURL,
		&link.CreatedAt,
		&link.Clicks,
		&link.LastAccessedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrLinkNotFound
		}
		return nil, fmt.Errorf("select link by url: %w", err)
	}

	return link, nil
}

// IncrementClicks увеличивает счётчик переходов и обновляет last_accessed_at
func (r *LinkRepository) IncrementClicks(ctx context.Context, id int64) error {
	query := `
		UPDATE links
		SET clicks = clicks + 1, last_accessed_at = $1
		WHERE id = $2
	`

	_, err := r.pool.Exec(ctx, query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("update clicks: %w", err)
	}

	return nil
}

// Delete удаляет ссылку по короткому коду
func (r *LinkRepository) Delete(ctx context.Context, code string) error {
	query := `DELETE FROM links WHERE short_code = $1`

	result, err := r.pool.Exec(ctx, query, code)
	if err != nil {
		return fmt.Errorf("delete link: %w", err)
	}

	// Проверяем, была ли удалена хотя бы одна строка
	if result.RowsAffected() == 0 {
		return model.ErrLinkNotFound
	}

	return nil
}

// isDuplicateError проверяет, является ли ошибка дубликатом уникального индекса
func isDuplicateError(err error) bool {
	// PostgreSQL error code 23505 = unique_violation
	// См. https://www.postgresql.org/docs/current/errcodes-appendix.html
	return err != nil && err.Error() != "" && contains(err.Error(), "23505")
}

// contains проверяет, содержит ли строка подстроку
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

// findSubstring ищет подстроку в строке
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
