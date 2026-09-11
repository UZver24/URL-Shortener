package redis

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"log/slog"
	"os"
)

func setupAnalyticsTest(t *testing.T) (*AnalyticsCache, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	cache := NewAnalyticsCache(client, logger)
	return cache, mr
}

// ==================== HyperLogLog тесты ====================

func TestAnalyticsCache_RecordUniqueClick(t *testing.T) {
	cache, mr := setupAnalyticsTest(t)
	defer mr.Close()

	ctx := context.Background()

	// Записываем уникальные клики
	for i := 0; i < 100; i++ {
		visitorID := string(rune('a'+i%26)) + string(rune('0'+i/26))
		err := cache.RecordUniqueClick(ctx, 1, visitorID)
		if err != nil {
			t.Fatalf("failed to record unique click: %v", err)
		}
	}

	// Получаем количество уникальных
	count, err := cache.GetUniqueClicks(ctx, 1)
	if err != nil {
		t.Fatalf("failed to get unique clicks: %v", err)
	}

	// HyperLogLog имеет погрешность ~0.81%, но для 100 элементов должно быть точно
	if count < 95 || count > 105 {
		t.Errorf("expected ~100 unique clicks, got %d", count)
	}
}

func TestAnalyticsCache_GetUniqueClicks_Empty(t *testing.T) {
	cache, mr := setupAnalyticsTest(t)
	defer mr.Close()

	ctx := context.Background()

	count, err := cache.GetUniqueClicks(ctx, 999)
	if err != nil {
		t.Fatalf("failed to get unique clicks: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 unique clicks for non-existent link, got %d", count)
	}
}

// ==================== Sorted Set тесты ====================

func TestAnalyticsCache_IncrementPopularity(t *testing.T) {
	cache, mr := setupAnalyticsTest(t)
	defer mr.Close()

	ctx := context.Background()

	// Инкрементируем популярность разных ссылок
	for i := 0; i < 10; i++ {
		err := cache.IncrementPopularity(ctx, 1, "abc123")
		if err != nil {
			t.Fatalf("failed to increment popularity: %v", err)
		}
	}

	for i := 0; i < 5; i++ {
		err := cache.IncrementPopularity(ctx, 2, "xyz789")
		if err != nil {
			t.Fatalf("failed to increment popularity: %v", err)
		}
	}

	// Получаем топ
	topLinks, err := cache.GetTopLinks(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get top links: %v", err)
	}

	if len(topLinks) != 2 {
		t.Errorf("expected 2 top links, got %d", len(topLinks))
	}

	if topLinks[0].ShortCode != "abc123" {
		t.Errorf("expected first link to be abc123, got %s", topLinks[0].ShortCode)
	}

	if topLinks[0].Clicks != 10 {
		t.Errorf("expected 10 clicks for abc123, got %d", topLinks[0].Clicks)
	}

	if topLinks[1].ShortCode != "xyz789" {
		t.Errorf("expected second link to be xyz789, got %s", topLinks[1].ShortCode)
	}

	if topLinks[1].Clicks != 5 {
		t.Errorf("expected 5 clicks for xyz789, got %d", topLinks[1].Clicks)
	}
}

func TestAnalyticsCache_GetTopLinks_Limit(t *testing.T) {
	cache, mr := setupAnalyticsTest(t)
	defer mr.Close()

	ctx := context.Background()

	// Создаём 20 ссылок с разной популярностью
	for i := 0; i < 20; i++ {
		for j := 0; j <= i; j++ {
			err := cache.IncrementPopularity(ctx, int64(i), string(rune('a'+i)))
			if err != nil {
				t.Fatalf("failed to increment popularity: %v", err)
			}
		}
	}

	// Запрашиваем топ-5
	topLinks, err := cache.GetTopLinks(ctx, 5)
	if err != nil {
		t.Fatalf("failed to get top links: %v", err)
	}

	if len(topLinks) != 5 {
		t.Errorf("expected 5 top links, got %d", len(topLinks))
	}

	// Проверяем, что топ отсортирован по убыванию
	for i := 1; i < len(topLinks); i++ {
		if topLinks[i].Clicks > topLinks[i-1].Clicks {
			t.Errorf("top links not sorted correctly: %d > %d", topLinks[i].Clicks, topLinks[i-1].Clicks)
		}
	}
}

// ==================== Sliding Window тесты ====================

func TestAnalyticsCache_RecordTrendingClick(t *testing.T) {
	cache, mr := setupAnalyticsTest(t)
	defer mr.Close()

	ctx := context.Background()

	// Записываем клики для trending
	for i := 0; i < 15; i++ {
		err := cache.RecordTrendingClick(ctx, 1, "abc123")
		if err != nil {
			t.Fatalf("failed to record trending click: %v", err)
		}
	}

	for i := 0; i < 10; i++ {
		err := cache.RecordTrendingClick(ctx, 2, "xyz789")
		if err != nil {
			t.Fatalf("failed to record trending click: %v", err)
		}
	}

	// Получаем trending
	trending, err := cache.GetTrendingLinks(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get trending links: %v", err)
	}

	if len(trending) != 2 {
		t.Errorf("expected 2 trending links, got %d", len(trending))
	}

	if trending[0].ShortCode != "abc123" {
		t.Errorf("expected first trending link to be abc123, got %s", trending[0].ShortCode)
	}

	if trending[0].Clicks != 15 {
		t.Errorf("expected 15 clicks for abc123, got %d", trending[0].Clicks)
	}
}

func TestAnalyticsCache_GetTrendingLinks_Empty(t *testing.T) {
	cache, mr := setupAnalyticsTest(t)
	defer mr.Close()

	ctx := context.Background()

	trending, err := cache.GetTrendingLinks(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get trending links: %v", err)
	}

	if len(trending) != 0 {
		t.Errorf("expected 0 trending links, got %d", len(trending))
	}
}
