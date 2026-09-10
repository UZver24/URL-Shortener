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
	kafkapkg "github.com/UZver24/URL-Shortener/internal/messaging/kafka"
	"github.com/UZver24/URL-Shortener/internal/repository/postgres"
	redisrepo "github.com/UZver24/URL-Shortener/internal/repository/redis"
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
	// 1. Настраиваем логирование
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// 2. Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger.Info("starting link service", "port", cfg.ServerPort)

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

	// 6. Подключаемся к Redis (graceful degradation)
	var redisClient *redisrepo.Client
	var linkCache service.LinkCache

	redisClient, err = redisrepo.NewClient(cfg.RedisAddr(), cfg.RedisPassword, cfg.RedisDB, logger)
	if err != nil {
		logger.Warn("failed to connect to Redis, running without cache", "error", err)
		redisClient = nil
	} else {
		ttl := time.Duration(cfg.RedisTTL) * time.Second
		linkCache = redisrepo.NewLinkCache(redisClient.GetClient(), logger, ttl)
		logger.Info("Redis cache enabled", "ttl", ttl)
	}

	// 7. Подключаемся к Kafka (producer для публикации событий кликов)
	kafkaProducer, err := kafkapkg.NewProducer(kafkapkg.ProducerConfig{
		Brokers: cfg.KafkaBrokers,
		Topic:   cfg.KafkaClicksTopic,
		Logger:  logger,
	})
	if err != nil {
		logger.Error("failed to create Kafka producer", "error", err)
		os.Exit(1)
	}
	defer kafkaProducer.Close()

	logger.Info("Kafka producer connected",
		"brokers", cfg.KafkaBrokers,
		"topic", cfg.KafkaClicksTopic,
	)

	// 8. Инициализируем слои приложения
	linkRepo := postgres.NewLinkRepository(pool)

	// Создаём publisher (Kafka)
	publisher := service.NewKafkaClickPublisher(kafkaProducer)

	// 9. Создаём сервис с кэшем и Kafka publisher
	linkService := service.NewLinkService(linkRepo, logger, linkCache, publisher)
	linkHandler := handler.NewLinkHandler(linkService)

	// Создаём HealthHandler
	var healthHandler *handler.HealthHandler
	if redisClient != nil {
		healthHandler = handler.NewHealthHandler(pool, redisClient)
	} else {
		healthHandler = handler.NewHealthHandler(pool, nil)
	}

	// 10. Настраиваем маршруты
	mux := http.NewServeMux()

	// Health-check и метрики
	mux.HandleFunc("GET /health", healthHandler.Health)
	mux.Handle("GET /metrics", promhttp.Handler())

	// API endpoints
	mux.HandleFunc("POST /api/v1/links", linkHandler.CreateLink)
	mux.HandleFunc("DELETE /api/v1/links/{short}", linkHandler.DeleteLink)

	// Redirect endpoint
	mux.HandleFunc("GET /{short}", linkHandler.Redirect)

	// 11. Применяем middleware
	handler := middleware.RequestID(middleware.Metrics(middleware.Logging(logger)(mux)))

	// 12. Создаём HTTP-сервер
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.ServerPort),
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 13. Запускаем сервер
	go func() {
		logger.Info("link service started", "port", cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// 14. Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info("received shutdown signal", "signal", sig.String())
	logger.Info("shutting down link service...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", "error", err)
	}

	if redisClient != nil {
		if err := redisClient.Close(); err != nil {
			logger.Error("failed to close Redis connection", "error", err)
		}
	}

	logger.Info("link service stopped gracefully")
}

// runMigrations применяет SQL-миграции
func runMigrations(databaseURL string) error {
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
