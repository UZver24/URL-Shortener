package redis

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/UZver24/URL-Shortener/internal/model"
	"github.com/alicebob/miniredis/v2"
	redisv9 "github.com/redis/go-redis/v9"
)

// testLogger — logger для тестов
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
}

// setupTestCache создаёт тестовый кэш с miniredis
func setupTestCache(t *testing.T) (*LinkCache, *miniredis.Miniredis) {
	// Запускаем мини-Redis
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	// Создаём клиент
	client := redisv9.NewClient(&redisv9.Options{
		Addr: mr.Addr(),
	})

	// Создаём кэш с TTL = 1 час
	ttl := time.Hour
	cache := NewLinkCache(client, testLogger(), ttl)

	return cache, mr
}

// TestCache_SetAndGet — успешное сохранение и получение
func TestCache_SetAndGet(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()

	// Создаём тестовую ссылку
	link := &model.Link{
		ID:          1,
		ShortCode:   "abc123",
		OriginalURL: "https://example.com",
		CreatedAt:   time.Now(),
		Clicks:      42,
	}

	// Сохраняем в кэш
	if err := cache.Set(ctx, link); err != nil {
		t.Fatalf("failed to set cache: %v", err)
	}

	// Получаем из кэша
	retrieved, err := cache.Get(ctx, "abc123")
	if err != nil {
		t.Fatalf("failed to get from cache: %v", err)
	}

	// Проверяем данные
	if retrieved.ID != link.ID {
		t.Errorf("expected ID %d, got %d", link.ID, retrieved.ID)
	}
	if retrieved.ShortCode != link.ShortCode {
		t.Errorf("expected ShortCode %s, got %s", link.ShortCode, retrieved.ShortCode)
	}
	if retrieved.OriginalURL != link.OriginalURL {
		t.Errorf("expected OriginalURL %s, got %s", link.OriginalURL, retrieved.OriginalURL)
	}
	if retrieved.Clicks != link.Clicks {
		t.Errorf("expected Clicks %d, got %d", link.Clicks, retrieved.Clicks)
	}
}

// TestCache_GetNotFound — получение несуществующего ключа
func TestCache_GetNotFound(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()

	// Пытаемся получить несуществующую ссылку
	_, err := cache.Get(ctx, "nonexistent")
	if err != model.ErrLinkNotFound {
		t.Errorf("expected ErrLinkNotFound, got %v", err)
	}
}

// TestCache_Delete — удаление из кэша
func TestCache_Delete(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()

	// Создаём и сохраняем ссылку
	link := &model.Link{
		ID:          1,
		ShortCode:   "delete123",
		OriginalURL: "https://example.com",
		CreatedAt:   time.Now(),
	}

	if err := cache.Set(ctx, link); err != nil {
		t.Fatalf("failed to set cache: %v", err)
	}

	// Проверяем, что ссылка есть
	_, err := cache.Get(ctx, "delete123")
	if err != nil {
		t.Fatalf("expected link to exist, got error: %v", err)
	}

	// Удаляем
	if err := cache.Delete(ctx, "delete123"); err != nil {
		t.Fatalf("failed to delete from cache: %v", err)
	}

	// Проверяем, что ссылка удалена
	_, err = cache.Get(ctx, "delete123")
	if err != model.ErrLinkNotFound {
		t.Errorf("expected ErrLinkNotFound after delete, got %v", err)
	}
}

// TestCache_TTL — проверка TTL
func TestCache_TTL(t *testing.T) {
	_, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()

	// Создаём кэш с коротким TTL (1 секунда)
	client := redisv9.NewClient(&redisv9.Options{
		Addr: mr.Addr(),
	})
	shortTTL := 1 * time.Second
	cacheShort := NewLinkCache(client, testLogger(), shortTTL)

	link := &model.Link{
		ID:          1,
		ShortCode:   "ttl123",
		OriginalURL: "https://example.com",
		CreatedAt:   time.Now(),
	}

	// Сохраняем
	if err := cacheShort.Set(ctx, link); err != nil {
		t.Fatalf("failed to set cache: %v", err)
	}

	// Сразу получаем — должно работать
	_, err := cacheShort.Get(ctx, "ttl123")
	if err != nil {
		t.Fatalf("expected link to exist immediately, got error: %v", err)
	}

	// Ждём 2 секунды (TTL должен истечь)
	mr.FastForward(2 * time.Second)

	// Пытаемся получить — должно быть ErrLinkNotFound
	_, err = cacheShort.Get(ctx, "ttl123")
	if err != model.ErrLinkNotFound {
		t.Errorf("expected ErrLinkNotFound after TTL, got %v", err)
	}
}

// TestCache_SetNil — сохранение nil не вызывает ошибку
func TestCache_SetNil(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()

	// Сохраняем nil — должно быть no-op
	if err := cache.Set(ctx, nil); err != nil {
		t.Errorf("expected no error for nil link, got %v", err)
	}
}
