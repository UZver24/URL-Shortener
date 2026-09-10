package redis

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func setupAnalyticsBench(b *testing.B) (*AnalyticsCache, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}

	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError + 1, // отключаем логирование в бенчмарках
	}))

	cache := NewAnalyticsCache(client, logger)
	return cache, mr
}

// ==================== HyperLogLog бенчмарки ====================

// BenchmarkRecordUniqueClick — бенчмарк записи уникального клика (PFADD)
// Сложность: O(1)
func BenchmarkRecordUniqueClick(b *testing.B) {
	cache, mr := setupAnalyticsBench(b)
	defer mr.Close()

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		visitorID := fmt.Sprintf("visitor_%d", i)
		_ = cache.RecordUniqueClick(ctx, 1, visitorID)
	}
}

// BenchmarkGetUniqueClicks — бенчмарк получения количества уникальных (PFCOUNT)
// Сложность: O(1)
func BenchmarkGetUniqueClicks(b *testing.B) {
	cache, mr := setupAnalyticsBench(b)
	defer mr.Close()

	ctx := context.Background()

	// Предзаполняем данными
	for i := 0; i < 10000; i++ {
		visitorID := fmt.Sprintf("visitor_%d", i)
		_ = cache.RecordUniqueClick(ctx, 1, visitorID)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.GetUniqueClicks(ctx, 1)
	}
}

// BenchmarkRecordUniqueClick_Parallel — параллельная запись уникальных кликов
func BenchmarkRecordUniqueClick_Parallel(b *testing.B) {
	cache, mr := setupAnalyticsBench(b)
	defer mr.Close()

	ctx := context.Background()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			visitorID := fmt.Sprintf("visitor_%d", i)
			_ = cache.RecordUniqueClick(ctx, 1, visitorID)
			i++
		}
	})
}

// ==================== Sorted Set бенчмарки ====================

// BenchmarkIncrementPopularity — бенчмарк инкремента популярности (ZINCRBY)
// Сложность: O(log N)
func BenchmarkIncrementPopularity(b *testing.B) {
	cache, mr := setupAnalyticsBench(b)
	defer mr.Close()

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		shortCode := fmt.Sprintf("code_%d", i%1000) // 1000 уникальных ссылок
		_ = cache.IncrementPopularity(ctx, int64(i%1000), shortCode)
	}
}

// BenchmarkGetTopLinks — бенчмарк получения топ-N ссылок (ZREVRANGE)
// Сложность: O(log(N) + M) где M = limit
func BenchmarkGetTopLinks(b *testing.B) {
	cache, mr := setupAnalyticsBench(b)
	defer mr.Close()

	ctx := context.Background()

	// Предзаполняем 10000 ссылок
	for i := 0; i < 10000; i++ {
		shortCode := fmt.Sprintf("code_%d", i)
		for j := 0; j < i%100; j++ { // разное количество кликов
			_ = cache.IncrementPopularity(ctx, int64(i), shortCode)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.GetTopLinks(ctx, 10)
	}
}

// BenchmarkGetTopLinks_100 — бенчмарк получения топ-100
func BenchmarkGetTopLinks_100(b *testing.B) {
	cache, mr := setupAnalyticsBench(b)
	defer mr.Close()

	ctx := context.Background()

	// Предзаполняем 10000 ссылок
	for i := 0; i < 10000; i++ {
		shortCode := fmt.Sprintf("code_%d", i)
		_ = cache.IncrementPopularity(ctx, int64(i), shortCode)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.GetTopLinks(ctx, 100)
	}
}

// ==================== Sliding Window бенчмарки ====================

// BenchmarkRecordTrendingClick — бенчмарк записи trending клика
// Сложность: O(1) (2 команды: ZINCRBY + EXPIRE в pipeline)
func BenchmarkRecordTrendingClick(b *testing.B) {
	cache, mr := setupAnalyticsBench(b)
	defer mr.Close()

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		shortCode := fmt.Sprintf("code_%d", i%1000)
		_ = cache.RecordTrendingClick(ctx, int64(i%1000), shortCode)
	}
}

// BenchmarkGetTrendingLinks — бенчмарк получения trending ссылок
// Сложность: O(M * log(N)) где M = 60 минут
func BenchmarkGetTrendingLinks(b *testing.B) {
	cache, mr := setupAnalyticsBench(b)
	defer mr.Close()

	ctx := context.Background()

	// Предзаполняем данные
	for i := 0; i < 1000; i++ {
		shortCode := fmt.Sprintf("code_%d", i)
		for j := 0; j < 10; j++ {
			_ = cache.RecordTrendingClick(ctx, int64(i), shortCode)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.GetTrendingLinks(ctx, 10)
	}
}
