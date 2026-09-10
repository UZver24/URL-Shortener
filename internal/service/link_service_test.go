package service

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/UZver24/URL-Shortener/internal/model"
)

// mockRepository — мок-репозиторий для тестов
type mockRepository struct {
	links          map[string]*model.Link // short_code → link
	linksByURL     map[string]*model.Link // original_url → link
	createErr      error
	getByCodeErr   error
	getByURLErr    error
	incrementErr   error
	deleteErr      error
	createCalls    int
	getCodeCalls   int
	getURLCalls    int
	incrementCalls int
	deleteCalls    int
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		links:      make(map[string]*model.Link),
		linksByURL: make(map[string]*model.Link),
	}
}

func (m *mockRepository) Create(ctx context.Context, link *model.Link) error {
	m.createCalls++
	if m.createErr != nil {
		return m.createErr
	}

	// Проверяем, существует ли уже такой short_code
	if _, exists := m.links[link.ShortCode]; exists {
		return model.ErrShortCodeAlreadyTaken
	}

	// Сохраняем
	link.ID = int64(len(m.links) + 1)
	m.links[link.ShortCode] = link
	m.linksByURL[link.OriginalURL] = link
	return nil
}

func (m *mockRepository) GetByCode(ctx context.Context, code string) (*model.Link, error) {
	m.getCodeCalls++
	if m.getByCodeErr != nil {
		return nil, m.getByCodeErr
	}

	link, exists := m.links[code]
	if !exists {
		return nil, model.ErrLinkNotFound
	}
	return link, nil
}

func (m *mockRepository) GetByOriginalURL(ctx context.Context, url string) (*model.Link, error) {
	m.getURLCalls++
	if m.getByURLErr != nil {
		return nil, m.getByURLErr
	}

	link, exists := m.linksByURL[url]
	if !exists {
		return nil, model.ErrLinkNotFound
	}
	return link, nil
}

func (m *mockRepository) IncrementClicks(ctx context.Context, id int64) error {
	m.incrementCalls++
	if m.incrementErr != nil {
		return m.incrementErr
	}

	for _, link := range m.links {
		if link.ID == id {
			link.Clicks++
			now := time.Now()
			link.LastAccessedAt = &now
			return nil
		}
	}
	return model.ErrLinkNotFound
}

func (m *mockRepository) Delete(ctx context.Context, code string) error {
	m.deleteCalls++
	if m.deleteErr != nil {
		return m.deleteErr
	}

	link, exists := m.links[code]
	if !exists {
		return model.ErrLinkNotFound
	}

	delete(m.links, code)
	delete(m.linksByURL, link.OriginalURL)
	return nil
}

// testLogger — logger для тестов (пишет в stderr)
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn, // только WARN и ERROR в тестах
	}))
}

// TestCreateLink_Success — успешное создание ссылки
func TestCreateLink_Success(t *testing.T) {
	repo := newMockRepository()
	svc := NewLinkService(repo, testLogger())

	link, err := svc.CreateLink(context.Background(), "https://example.com", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if link == nil {
		t.Fatal("expected link, got nil")
	}

	if link.OriginalURL != "https://example.com" {
		t.Errorf("expected original URL 'https://example.com', got '%s'", link.OriginalURL)
	}

	if len(link.ShortCode) != 6 {
		t.Errorf("expected short code length 6, got %d", len(link.ShortCode))
	}

	if repo.createCalls != 1 {
		t.Errorf("expected 1 create call, got %d", repo.createCalls)
	}
}

// TestCreateLink_InvalidURL — невалидный URL
func TestCreateLink_InvalidURL(t *testing.T) {
	repo := newMockRepository()
	svc := NewLinkService(repo, testLogger())

	testCases := []string{
		"",
		"not-a-url",
		"ftp://",
		"http://",
	}

	for _, url := range testCases {
		_, err := svc.CreateLink(context.Background(), url, "")
		if !errors.Is(err, model.ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for URL '%s', got %v", url, err)
		}
	}

	if repo.createCalls != 0 {
		t.Errorf("expected 0 create calls for invalid URLs, got %d", repo.createCalls)
	}
}

// TestCreateLink_DuplicateURL — повторное создание того же URL
func TestCreateLink_DuplicateURL(t *testing.T) {
	repo := newMockRepository()
	svc := NewLinkService(repo, testLogger())

	// Первый запрос
	link1, err := svc.CreateLink(context.Background(), "https://example.com", "")
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	// Второй запрос (тот же URL)
	link2, err := svc.CreateLink(context.Background(), "https://example.com", "")
	if err != nil {
		t.Fatalf("second create failed: %v", err)
	}

	// Должны получить ту же ссылку
	if link1.ShortCode != link2.ShortCode {
		t.Errorf("expected same short code, got '%s' and '%s'", link1.ShortCode, link2.ShortCode)
	}

	// Repository.Create должен был вызваться только 1 раз
	if repo.createCalls != 1 {
		t.Errorf("expected 1 create call (duplicate returned early), got %d", repo.createCalls)
	}
}

// TestCreateLink_CustomCode — пользовательский short_code
func TestCreateLink_CustomCode(t *testing.T) {
	repo := newMockRepository()
	svc := NewLinkService(repo, testLogger())

	link, err := svc.CreateLink(context.Background(), "https://example.com", "custom")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if link.ShortCode != "custom" {
		t.Errorf("expected short code 'custom', got '%s'", link.ShortCode)
	}
}

// TestCreateLink_Collision — коллизия short_code и retry
func TestCreateLink_Collision(t *testing.T) {
	repo := newMockRepository()
	svc := NewLinkService(repo, testLogger())

	// Создаём первую ссылку
	_, err := svc.CreateLink(context.Background(), "https://example1.com", "abc123")
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	// Пытаемся создать вторую с тем же custom_code (должна быть коллизия)
	// Но сервис должен сгенерировать новый код и succeed
	link2, err := svc.CreateLink(context.Background(), "https://example2.com", "abc123")
	if err != nil {
		t.Fatalf("second create failed: %v", err)
	}

	// Short code должен быть другим (сгенерированным)
	if link2.ShortCode == "abc123" {
		t.Errorf("expected different short code after collision, got 'abc123'")
	}

	// Должно быть 3 вызова Create:
	// 1. Первая ссылка (abc123) — успешно
	// 2. Вторая ссылка с abc123 — коллизия
	// 3. Вторая ссылка с новым кодом — успешно
	if repo.createCalls != 3 {
		t.Errorf("expected 3 create calls (first link + collision + retry), got %d", repo.createCalls)
	}
}

// TestGetOriginalURL_Success — успешное получение URL
func TestGetOriginalURL_Success(t *testing.T) {
	repo := newMockRepository()
	svc := NewLinkService(repo, testLogger())

	// Создаём ссылку
	_, _ = svc.CreateLink(context.Background(), "https://example.com", "test123")

	// Получаем URL
	url, err := svc.GetOriginalURL(context.Background(), "test123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if url != "https://example.com" {
		t.Errorf("expected 'https://example.com', got '%s'", url)
	}

	// Даём время goroutine для increment
	time.Sleep(10 * time.Millisecond)

	// Проверяем, что clicks увеличился
	link, _ := repo.GetByCode(context.Background(), "test123")
	if link.Clicks != 1 {
		t.Errorf("expected clicks=1, got %d", link.Clicks)
	}

	if link.LastAccessedAt == nil {
		t.Error("expected LastAccessedAt to be set")
	}
}

// TestGetOriginalURL_NotFound — ссылка не найдена
func TestGetOriginalURL_NotFound(t *testing.T) {
	repo := newMockRepository()
	svc := NewLinkService(repo, testLogger())

	_, err := svc.GetOriginalURL(context.Background(), "nonexistent")
	if !errors.Is(err, model.ErrLinkNotFound) {
		t.Errorf("expected ErrLinkNotFound, got %v", err)
	}
}

// TestGetStats_Success — получение статистики
func TestGetStats_Success(t *testing.T) {
	repo := newMockRepository()
	svc := NewLinkService(repo, testLogger())

	// Создаём ссылку
	_, _ = svc.CreateLink(context.Background(), "https://example.com", "stats123")

	// Получаем статистику
	stats, err := svc.GetStats(context.Background(), "stats123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if stats.ShortCode != "stats123" {
		t.Errorf("expected short code 'stats123', got '%s'", stats.ShortCode)
	}

	if stats.Clicks != 0 {
		t.Errorf("expected clicks=0, got %d", stats.Clicks)
	}
}

// TestDeleteLink_Success — успешное удаление
func TestDeleteLink_Success(t *testing.T) {
	repo := newMockRepository()
	svc := NewLinkService(repo, testLogger())

	// Создаём ссылку
	_, _ = svc.CreateLink(context.Background(), "https://example.com", "delete123")

	// Удаляем
	err := svc.DeleteLink(context.Background(), "delete123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Проверяем, что ссылка удалена
	_, err = repo.GetByCode(context.Background(), "delete123")
	if !errors.Is(err, model.ErrLinkNotFound) {
		t.Errorf("expected ErrLinkNotFound after delete, got %v", err)
	}

	if repo.deleteCalls != 1 {
		t.Errorf("expected 1 delete call, got %d", repo.deleteCalls)
	}
}

// TestDeleteLink_NotFound — удаление несуществующей ссылки
func TestDeleteLink_NotFound(t *testing.T) {
	repo := newMockRepository()
	svc := NewLinkService(repo, testLogger())

	err := svc.DeleteLink(context.Background(), "nonexistent")
	if !errors.Is(err, model.ErrLinkNotFound) {
		t.Errorf("expected ErrLinkNotFound, got %v", err)
	}
}

// TestIsValidURL — тестирование валидации URL
func TestIsValidURL(t *testing.T) {
	validURLs := []string{
		"https://example.com",
		"http://example.com/path?query=value",
		"https://sub.example.com:8080/path",
		"http://localhost:3000",
	}

	invalidURLs := []string{
		"",
		"not-a-url",
		"ftp://",
		"http://",
		"https://",
		"example.com", // нет схемы
	}

	for _, url := range validURLs {
		if !isValidURL(url) {
			t.Errorf("expected '%s' to be valid", url)
		}
	}

	for _, url := range invalidURLs {
		if isValidURL(url) {
			t.Errorf("expected '%s' to be invalid", url)
		}
	}
}

// TestGenerateShortCode — тестирование генерации кода
func TestGenerateShortCode(t *testing.T) {
	codes := make(map[string]bool)

	// Генерируем 1000 кодов и проверяем уникальность
	for i := 0; i < 1000; i++ {
		code, err := generateShortCode()
		if err != nil {
			t.Fatalf("generateShortCode failed: %v", err)
		}

		if len(code) != 6 {
			t.Errorf("expected length 6, got %d", len(code))
		}

		if codes[code] {
			t.Errorf("duplicate code generated: %s", code)
		}
		codes[code] = true
	}
}
