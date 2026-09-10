package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/url"
	"time"

	"github.com/UZver24/URL-Shortener/internal/model"
)

// LinkRepository — интерфейс для работы с ссылками
type LinkRepository interface {
	Create(ctx context.Context, link *model.Link) error
	GetByCode(ctx context.Context, code string) (*model.Link, error)
	GetByOriginalURL(ctx context.Context, originalURL string) (*model.Link, error)
	IncrementClicks(ctx context.Context, id int64) error
	Delete(ctx context.Context, code string) error
}

// LinkService реализует бизнес-логику для работы со ссылками
type LinkService struct {
	repo LinkRepository
}

// NewLinkService создаёт новый сервис
func NewLinkService(repo LinkRepository) *LinkService {
	return &LinkService{repo: repo}
}

// CreateLink создаёт новую короткую ссылку
func (s *LinkService) CreateLink(ctx context.Context, originalURL string, customCode string) (*model.Link, error) {
	// Валидация URL
	if !isValidURL(originalURL) {
		return nil, model.ErrInvalidInput
	}

	// Проверяем, существует ли уже ссылка с таким original_url
	existing, err := s.repo.GetByOriginalURL(ctx, originalURL)
	if err == nil && existing != nil {
		// Ссылка уже существует — возвращаем её
		return existing, nil
	}

	// Генерируем short_code
	var shortCode string
	if customCode != "" {
		shortCode = customCode
	} else {
		shortCode, err = generateShortCode()
		if err != nil {
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
			return link, nil
		}

		if err == model.ErrShortCodeAlreadyTaken {
			// Коллизия — генерируем новый код и пробуем снова
			shortCode, err = generateShortCode()
			if err != nil {
				return nil, fmt.Errorf("generate short code: %w", err)
			}
			link.ShortCode = shortCode
			continue
		}

		// Другая ошибка — возвращаем
		return nil, err
	}

	return nil, fmt.Errorf("failed to create link after 3 attempts")
}

// GetOriginalURL возвращает оригинальный URL по короткому коду и увеличивает счётчик
func (s *LinkService) GetOriginalURL(ctx context.Context, code string) (string, error) {
	link, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return "", err
	}

	// Асинхронно увеличиваем счётчик (не блокируем ответ)
	go func() {
		// Используем background context, так как оригинальный может быть отменён
		bgCtx := context.Background()
		_ = s.repo.IncrementClicks(bgCtx, link.ID)
	}()

	return link.OriginalURL, nil
}

// GetStats возвращает статистику по ссылке
func (s *LinkService) GetStats(ctx context.Context, code string) (*model.Link, error) {
	return s.repo.GetByCode(ctx, code)
}

// DeleteLink удаляет ссылку
func (s *LinkService) DeleteLink(ctx context.Context, code string) error {
	return s.repo.Delete(ctx, code)
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
