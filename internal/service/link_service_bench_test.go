package service

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

// benchLogger — логгер для бенчмарков (минимальный вывод)
func benchLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError + 1, // отключаем логирование в бенчмарках
	}))
}

// BenchmarkGenerateShortCode — бенчмарк текущей реализации (crypto/rand, 6 символов)
func BenchmarkGenerateShortCode(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := generateShortCode()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGenerateShortCode_Parallel — параллельная генерация
func BenchmarkGenerateShortCode_Parallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := generateShortCode()
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkIsValidURL — бенчмарк валидации URL
func BenchmarkIsValidURL(b *testing.B) {
	urls := []string{
		"https://example.com",
		"http://example.com/very/long/path/with/many/segments?query=value&other=123",
		"https://sub.example.com:8080/path",
		"",
		"not-a-url",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, u := range urls {
			isValidURL(u)
		}
	}
}

// BenchmarkCreateLink — бенчмарк создания ссылки (с mock-репозиторием)
func BenchmarkCreateLink(b *testing.B) {
	repo := newMockRepository()
	svc := NewLinkService(repo, benchLogger(), nil, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := svc.CreateLink(context.Background(), "https://example.com/bench", "")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetOriginalURL_NoCache — бенчмарк redirect без кэша
func BenchmarkGetOriginalURL_NoCache(b *testing.B) {
	repo := newMockRepository()
	svc := NewLinkService(repo, benchLogger(), nil, nil)

	// Создаём ссылку
	link, err := svc.CreateLink(context.Background(), "https://example.com/bench", "bench123")
	if err != nil {
		b.Fatal(err)
	}
	_ = link

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := svc.GetOriginalURL(context.Background(), "bench123", "", "")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetOriginalURL_WithCache — бенчмарк redirect с кэшем (cache hit)
func BenchmarkGetOriginalURL_WithCache(b *testing.B) {
	repo := newMockRepository()
	cache := newMockCache()
	svc := NewLinkService(repo, benchLogger(), cache, nil)

	// Создаём ссылку
	link, err := svc.CreateLink(context.Background(), "https://example.com/bench", "bench_cache")
	if err != nil {
		b.Fatal(err)
	}

	// Предзаполняем кэш
	cache.data["bench_cache"] = link

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := svc.GetOriginalURL(context.Background(), "bench_cache", "", "")
		if err != nil {
			b.Fatal(err)
		}
	}
}
