package main

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/UZver24/URL-Shortener/internal/config"
	"github.com/UZver24/URL-Shortener/internal/handler"
	"github.com/UZver24/URL-Shortener/internal/handler/middleware"
	"github.com/UZver24/URL-Shortener/internal/repository/postgres"
	"github.com/UZver24/URL-Shortener/internal/service"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	// 1. Настраиваем структурированное логирование
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger) // глобальный logger по умолчанию

	// 2. Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// 3. Подключаемся к PostgreSQL
	pool, err := postgres.NewPool(cfg.DatabaseURL())
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	logger.Info("connected to PostgreSQL",
		"host", cfg.PostgresHost,
		"port", cfg.PostgresPort,
		"database", cfg.PostgresDB,
	)

	// 4. Запускаем миграции
	if err := runMigrations(cfg.DatabaseURL()); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	logger.Info("migrations completed")

	// 5. Регистрируем метрики БД
	registerDBMetrics(pool)

	// 6. Инициализируем слои приложения
	linkRepo := postgres.NewLinkRepository(pool)
	linkService := service.NewLinkService(linkRepo, logger)
	linkHandler := handler.NewLinkHandler(linkService)
	healthHandler := handler.NewHealthHandler(pool)

	// 7. Настраиваем маршруты
	mux := http.NewServeMux()

	// Health-check и метрики
	mux.HandleFunc("GET /health", healthHandler.Health)
	mux.Handle("GET /metrics", promhttp.Handler())

	// API endpoints
	mux.HandleFunc("POST /api/v1/links", linkHandler.CreateLink)
	mux.HandleFunc("GET /api/v1/links/{short}/stats", linkHandler.GetStats)
	mux.HandleFunc("DELETE /api/v1/links/{short}", linkHandler.DeleteLink)

	// Redirect endpoint (должен быть последним, так как перехватывает все GET /{short})
	mux.HandleFunc("GET /{short}", linkHandler.Redirect)

	// 8. Применяем middleware в правильном порядке (снаружи → внутрь):
	//    Запрос → RequestID → Metrics → Logging → mux → Handler → Ответ
	handler := middleware.RequestID(middleware.Metrics(middleware.Logging(logger)(mux)))

	// 8. Создаём HTTP-сервер
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.ServerPort),
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 9. Запускаем сервер в отдельной горутине
	go func() {
		logger.Info("server started", "port", cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// 10. Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info("received shutdown signal", "signal", sig.String())
	logger.Info("shutting down server...")

	// Даём 30 секунд на завершение активных запросов
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped gracefully")
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

// registerDBMetrics регистрирует метрики для пула соединений БД
func registerDBMetrics(pool *pgxpool.Pool) {
	promauto.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "db_pool_active_connections",
			Help: "Number of active database connections",
		},
		func() float64 {
			stat := pool.Stat()
			return float64(stat.TotalConns() - stat.IdleConns())
		},
	)

	promauto.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "db_pool_idle_connections",
			Help: "Number of idle database connections",
		},
		func() float64 {
			stat := pool.Stat()
			return float64(stat.IdleConns())
		},
	)

	promauto.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "db_pool_total_connections",
			Help: "Total number of database connections in pool",
		},
		func() float64 {
			stat := pool.Stat()
			return float64(stat.TotalConns())
		},
	)
}
