package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/UZver24/URL-Shortener/internal/config"
	"github.com/UZver24/URL-Shortener/internal/handler"
	"github.com/UZver24/URL-Shortener/internal/repository/postgres"
	"github.com/UZver24/URL-Shortener/internal/service"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	// 1. Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 2. Подключаемся к PostgreSQL
	pool, err := postgres.NewPool(cfg.DatabaseURL())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	log.Println("Connected to PostgreSQL")

	// 3. Запускаем миграции
	if err := runMigrations(cfg.DatabaseURL()); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	log.Println("Migrations completed")

	// 4. Инициализируем слои приложения
	linkRepo := postgres.NewLinkRepository(pool)
	linkService := service.NewLinkService(linkRepo)
	linkHandler := handler.NewLinkHandler(linkService)

	// 5. Настраиваем маршруты
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("POST /api/v1/links", linkHandler.CreateLink)
	mux.HandleFunc("GET /api/v1/links/{short}/stats", linkHandler.GetStats)
	mux.HandleFunc("DELETE /api/v1/links/{short}", linkHandler.DeleteLink)

	// Redirect endpoint (должен быть последним, так как перехватывает все GET /{short})
	mux.HandleFunc("GET /{short}", linkHandler.Redirect)

	// 6. Создаём HTTP-сервер
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.ServerPort),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 7. Запускаем сервер в отдельной горутине
	go func() {
		log.Printf("Server started on port %d", cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// 8. Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Даём 30 секунд на завершение активных запросов
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}

// runMigrations применяет SQL-миграции
func runMigrations(databaseURL string) error {
	// Используем embed.FS как источник миграций
	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create source driver: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", sourceDriver, databaseURL)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
