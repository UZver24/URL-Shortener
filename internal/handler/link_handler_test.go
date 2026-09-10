package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/UZver24/URL-Shortener/internal/model"
	"github.com/UZver24/URL-Shortener/internal/service"
)

// mockRepository — мок-репозиторий для тестов
type mockRepository struct {
	links      map[string]*model.Link // short_code → link
	linksByURL map[string]*model.Link // original_url → link
	idCounter  int64
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		links:      make(map[string]*model.Link),
		linksByURL: make(map[string]*model.Link),
	}
}

func (m *mockRepository) Create(ctx context.Context, link *model.Link) error {
	// Проверяем, существует ли уже такой short_code
	if _, exists := m.links[link.ShortCode]; exists {
		return model.ErrShortCodeAlreadyTaken
	}

	// Сохраняем
	m.idCounter++
	link.ID = m.idCounter
	m.links[link.ShortCode] = link
	m.linksByURL[link.OriginalURL] = link
	return nil
}

func (m *mockRepository) GetByCode(ctx context.Context, code string) (*model.Link, error) {
	link, exists := m.links[code]
	if !exists {
		return nil, model.ErrLinkNotFound
	}
	return link, nil
}

func (m *mockRepository) GetByOriginalURL(ctx context.Context, url string) (*model.Link, error) {
	link, exists := m.linksByURL[url]
	if !exists {
		return nil, model.ErrLinkNotFound
	}
	return link, nil
}

func (m *mockRepository) IncrementClicks(ctx context.Context, id int64) error {
	for _, link := range m.links {
		if link.ID == id {
			link.Clicks++
			return nil
		}
	}
	return model.ErrLinkNotFound
}

func (m *mockRepository) Delete(ctx context.Context, code string) error {
	link, exists := m.links[code]
	if !exists {
		return model.ErrLinkNotFound
	}

	delete(m.links, code)
	delete(m.linksByURL, link.OriginalURL)
	return nil
}

// setupTestServer создаёт тестовый сервер с моком
func setupTestServer() (*LinkHandler, *mockRepository) {
	repo := newMockRepository()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError, // только ошибки в тестах
	}))
	svc := service.NewLinkService(repo, logger, nil) // без кэша
	handler := NewLinkHandler(svc)
	return handler, repo
}

// TestCreateLink_Success — успешное создание ссылки
func TestCreateLink_Success(t *testing.T) {
	handler, _ := setupTestServer()

	// Создаём запрос
	reqBody := model.CreateLinkRequest{
		URL: "https://example.com",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/links", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	// Выполняем запрос
	w := httptest.NewRecorder()
	handler.CreateLink(w, req)

	// Проверяем ответ
	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201, got %d", resp.StatusCode)
	}

	var response model.CreateLinkResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Original != "https://example.com" {
		t.Errorf("expected original 'https://example.com', got '%s'", response.Original)
	}

	if len(response.Short) != 6 {
		t.Errorf("expected short code length 6, got %d", len(response.Short))
	}
}

// TestCreateLink_InvalidJSON — невалидный JSON
func TestCreateLink_InvalidJSON(t *testing.T) {
	handler, _ := setupTestServer()

	req := httptest.NewRequest("POST", "/api/v1/links", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.CreateLink(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
}

// TestCreateLink_InvalidURL — невалидный URL
func TestCreateLink_InvalidURL(t *testing.T) {
	handler, _ := setupTestServer()

	reqBody := model.CreateLinkRequest{
		URL: "not-a-url",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/links", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.CreateLink(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
}

// TestRedirect_Success — успешный редирект
func TestRedirect_Success(t *testing.T) {
	handler, repo := setupTestServer()

	// Создаём ссылку напрямую в репозиторий
	repo.links["abc123"] = &model.Link{
		ID:          1,
		ShortCode:   "abc123",
		OriginalURL: "https://example.com",
		Clicks:      0,
	}

	req := httptest.NewRequest("GET", "/abc123", nil)
	w := httptest.NewRecorder()

	handler.Redirect(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected status 302, got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "https://example.com" {
		t.Errorf("expected Location 'https://example.com', got '%s'", location)
	}
}

// TestRedirect_NotFound — ссылка не найдена
func TestRedirect_NotFound(t *testing.T) {
	handler, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.Redirect(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

// TestRedirect_EmptyCode — пустой short code
func TestRedirect_EmptyCode(t *testing.T) {
	handler, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.Redirect(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
}

// TestGetStats_Success — успешное получение статистики
func TestGetStats_Success(t *testing.T) {
	handler, repo := setupTestServer()

	// Создаём ссылку напрямую в репозиторий
	repo.links["stats123"] = &model.Link{
		ID:          1,
		ShortCode:   "stats123",
		OriginalURL: "https://example.com",
		Clicks:      42,
	}

	req := httptest.NewRequest("GET", "/api/v1/links/stats123/stats", nil)
	w := httptest.NewRecorder()

	handler.GetStats(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var response model.LinkStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Short != "stats123" {
		t.Errorf("expected short 'stats123', got '%s'", response.Short)
	}

	if response.Clicks != 42 {
		t.Errorf("expected clicks 42, got %d", response.Clicks)
	}
}

// TestGetStats_NotFound — ссылка не найдена
func TestGetStats_NotFound(t *testing.T) {
	handler, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/links/nonexistent/stats", nil)
	w := httptest.NewRecorder()

	handler.GetStats(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

// TestDeleteLink_Success — успешное удаление
func TestDeleteLink_Success(t *testing.T) {
	handler, repo := setupTestServer()

	// Создаём ссылку напрямую в репозиторий
	repo.links["delete123"] = &model.Link{
		ID:          1,
		ShortCode:   "delete123",
		OriginalURL: "https://example.com",
	}

	req := httptest.NewRequest("DELETE", "/api/v1/links/delete123", nil)
	w := httptest.NewRecorder()

	handler.DeleteLink(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", resp.StatusCode)
	}

	// Проверяем, что ссылка удалена
	if _, exists := repo.links["delete123"]; exists {
		t.Error("expected link to be deleted")
	}
}

// TestDeleteLink_NotFound — ссылка не найдена
func TestDeleteLink_NotFound(t *testing.T) {
	handler, _ := setupTestServer()

	req := httptest.NewRequest("DELETE", "/api/v1/links/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.DeleteLink(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

// TestCreateLink_DuplicateURL — повторное создание того же URL
func TestCreateLink_DuplicateURL(t *testing.T) {
	handler, _ := setupTestServer()

	// Первый запрос
	reqBody := model.CreateLinkRequest{
		URL: "https://example.com",
	}
	body, _ := json.Marshal(reqBody)

	req1 := httptest.NewRequest("POST", "/api/v1/links", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	handler.CreateLink(w1, req1)

	var resp1 model.CreateLinkResponse
	json.NewDecoder(w1.Result().Body).Decode(&resp1)

	// Второй запрос (тот же URL)
	req2 := httptest.NewRequest("POST", "/api/v1/links", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	handler.CreateLink(w2, req2)

	var resp2 model.CreateLinkResponse
	json.NewDecoder(w2.Result().Body).Decode(&resp2)

	// Должны получить ту же ссылку
	if resp1.Short != resp2.Short {
		t.Errorf("expected same short code, got '%s' and '%s'", resp1.Short, resp2.Short)
	}
}
