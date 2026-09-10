package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/url"
	"time"

	"github.com/UZver24/URL-Shortener/internal/handler/middleware"
	"github.com/UZver24/URL-Shortener/internal/model"
	"github.com/UZver24/URL-Shortener/internal/worker"
)

// LinkRepository — интерфейс для работы с ссылками
type LinkRepository interface {
	Create(ctx context.Context, link *model.Link) error
	GetByCode(ctx context.Context, code string) (*model.Link, error)
	GetByOriginalURL(ctx context.Context, originalURL string) (*model.Link, error)
	IncrementClicks(ctx context.Context, id int64) error
	Delete(ctx context.Context, code string) error
}

// LinkCache — интерфейс для кэш-слоя
type LinkCache interface {
	Get(ctx context.Context, shortCode string) (*model.Link, error)
	Set(ctx context.Context, link *model.Link) error
	Delete(ctx context.Context, shortCode string) error
}

// LinkService реализует бизнес-логику для работы со ссылками
type LinkService struct {
	repo   LinkRepository
	cache  LinkCache          // опциональный кэш (может быть nil)
	pool   *worker.WorkerPool // опциональный воркер-пул (может быть nil)
	logger *slog.Logger
}

// NewLinkService создаёт новый сервис
// cache и pool могут быть nil — тогда работаем без кэша/воркер-пула
func NewLinkService(repo LinkRepository, logger *slog.Logger, cache LinkCache, pool *worker.WorkerPool) *LinkService {
	return &LinkService{
		repo:   repo,
		cache:  cache,
		pool:   pool,
		logger: logger,
	}
}

// CreateLink создаёт новую короткую ссылку
func (s *LinkService) CreateLink(ctx context.Context, originalURL string, customCode string) (*model.Link, error) {
	// Валидация URL
	if !isValidURL(originalURL) {
		s.logger.Warn("invalid URL provided", "url", originalURL)
		return nil, model.ErrInvalidInput
	}

	// Проверяем, существует ли уже ссылка с таким original_url
	existing, err := s.repo.GetByOriginalURL(ctx, originalURL)
	if err == nil && existing != nil {
		s.logger.Info("returning existing link",
			"original_url", originalURL,
			"short_code", existing.ShortCode,
		)
		return existing, nil
	}

	// Генерируем short_code
	var shortCode string
	if customCode != "" {
		shortCode = customCode
	} else {
		shortCode, err = generateShortCode()
		if err != nil {
			s.logger.Error("failed to generate short code", "error", err)
			return nil, fmt.Errorf("generate short code: %w", err)
		}
	}

	// Создаём ссылку
	link := &model.Link{
		ShortCode:   shortCode,
		OriginalURL: originalURL,
		CreatedAt:   time.Now(),
		Clicks:      0,
	}

	// Пытаемся сохранить (может быть коллизия short_code)
	for attempts := 0; attempts < 3; attempts++ {
		err = s.repo.Create(ctx, link)
		if err == nil {
			s.logger.Info("link created",
				"short_code", link.ShortCode,
				"original_url", link.OriginalURL,
				"id", link.ID,
			)
			return link, nil
		}

		if err == model.ErrShortCodeAlreadyTaken {
			s.logger.Warn("short code collision, retrying",
				"short_code", shortCode,
				"attempt", attempts+1,
			)
			// Коллизия — генерируем новый код и пробуем снова
			shortCode, err = generateShortCode()
			if err != nil {
				s.logger.Error("failed to generate short code on retry", "error", err)
				return nil, fmt.Errorf("generate short code: %w", err)
			}
			link.ShortCode = shortCode
			continue
		}

		// Другая ошибка — возвращаем
		s.logger.Error("failed to create link", "error", err)
		return nil, err
	}

	s.logger.Error("failed to create link after 3 attempts")
	return nil, fmt.Errorf("failed to create link after 3 attempts")
}

// GetOriginalURL возвращает оригинальный URL по короткому коду и увеличивает счётчик
// Реализует cache-aside паттерн: сначала проверяем кэш, потом БД
func (s *LinkService) GetOriginalURL(ctx context.Context, code string) (string, error) {
	var link *model.Link
	var err error

	// 1. Пробуем получить из кэша (если кэш доступен)
	if s.cache != nil {
		link, err = s.cache.Get(ctx, code)
		if err == nil {
			// Cache hit — используем данные из кэша
			s.logger.Debug("cache hit", "short_code", code)
			middleware.RecordCacheHit("get")
			// Асинхронно увеличиваем счётчик через воркер-пул
			s.incrementClicksAsync(link.ID, link.ShortCode)
			return link.OriginalURL, nil
		}
		// Cache miss или ошибка — продолжаем с БД
		if errors.Is(err, model.ErrLinkNotFound) {
			// Это cache miss
			middleware.RecordCacheMiss("get")
		} else {
			// Это ошибка кэша
			s.logger.Warn("cache get failed", "short_code", code, "error", err)
			middleware.RecordCacheError("get")
		}
	}

	// 2. Получаем из БД
	link, err = s.repo.GetByCode(ctx, code)
	if err != nil {
		return "", err
	}

	// 3. Сохраняем в кэш (ленивое кэширование)
	if s.cache != nil {
		bgCtx := context.Background()
		if err := s.cache.Set(bgCtx, link); err != nil {
			s.logger.Warn("cache set failed", "short_code", code, "error", err)
			middleware.RecordCacheError("set")
		}
	}

	// 4. Асинхронно увеличиваем счётчик через воркер-пул
	s.incrementClicksAsync(link.ID, link.ShortCode)

	return link.OriginalURL, nil
}

// incrementClicksAsync увеличивает счётчик переходов через воркер-пул
// Если pool недоступен — fallback к простому goroutine (для совместимости)
func (s *LinkService) incrementClicksAsync(linkID int64, shortCode string) {
	if s.pool != nil {
		task := worker.NewTask(linkID, shortCode)
		if err := s.pool.Submit(task); err != nil {
			s.logger.Warn("failed to submit task to worker pool",
				"link_id", linkID,
				"short_code", shortCode,
				"error", err,
			)
			// Fallback: синхронный increment в фоне
			go s.syncIncrementClicks(linkID)
		}
		return
	}

	// Fallback: без воркер-пула (для тестов или graceful degradation)
	go s.syncIncrementClicks(linkID)
}

// syncIncrementClicks — синхронное увеличение счётчика (fallback)
func (s *LinkService) syncIncrementClicks(linkID int64) {
	bgCtx := context.Background()
	if err := s.repo.IncrementClicks(bgCtx, linkID); err != nil {
		s.logger.Error("failed to increment clicks",
			"link_id", linkID,
			"error", err,
		)
	}
}

// GetStats возвращает статистику по ссылке
// Также использует cache-aside паттерн
func (s *LinkService) GetStats(ctx context.Context, code string) (*model.Link, error) {
	var link *model.Link
	var err error

	// 1. Пробуем получить из кэша
	if s.cache != nil {
		link, err = s.cache.Get(ctx, code)
		if err == nil {
			s.logger.Debug("cache hit for stats", "short_code", code)
			middleware.RecordCacheHit("get")
			return link, nil
		}
		if errors.Is(err, model.ErrLinkNotFound) {
			middleware.RecordCacheMiss("get")
		} else {
			s.logger.Warn("cache get failed for stats", "short_code", code, "error", err)
			middleware.RecordCacheError("get")
		}
	}

	// 2. Получаем из БД
	link, err = s.repo.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	// 3. Сохраняем в кэш
	if s.cache != nil {
		bgCtx := context.Background()
		if err := s.cache.Set(bgCtx, link); err != nil {
			s.logger.Warn("cache set failed for stats", "short_code", code, "error", err)
			middleware.RecordCacheError("set")
		}
	}

	return link, nil
}

// DeleteLink удаляет ссылку и инвалидирует кэш
func (s *LinkService) DeleteLink(ctx context.Context, code string) error {
	// 1. Удаляем из БД
	err := s.repo.Delete(ctx, code)
	if err != nil {
		return err
	}

	// 2. Инвалидируем кэш
	if s.cache != nil {
		bgCtx := context.Background()
		if err := s.cache.Delete(bgCtx, code); err != nil {
			s.logger.Warn("cache delete failed", "short_code", code, "error", err)
			middleware.RecordCacheError("delete")
		}
	}

	s.logger.Info("link deleted", "short_code", code)
	return nil
}

// isValidURL проверяет, является ли строка валидным URL
func isValidURL(str string) bool {
	if str == "" {
		return false
	}

	u, err := url.Parse(str)
	if err != nil {
		return false
	}

	// Должна быть схема (http/https) и хост
	return u.Scheme != "" && u.Host != ""
}

// generateShortCode генерирует случайный код из 6 символов (base62)
func generateShortCode() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 6

	code := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		code[i] = charset[n.Int64()]
	}

	return string(code), nil
}
