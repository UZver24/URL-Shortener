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
	"github.com/UZver24/URL-Shortener/internal/repository/postgres/stats"
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

	logger.Info("starting stats service", "port", cfg.ServerPort)

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

	// 4. Запускаем миграции (stats tables)
	if err := runMigrations(cfg.DatabaseURL()); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	logger.Info("migrations completed")

	// 5. Регистрируем метрики БД
	registerDBMetrics(pool)

	// 6. Подключаемся к Redis (для аналитики: HyperLogLog, Sorted Set, Sliding Window)
	var analyticsCache service.AnalyticsCache
	redisClient, err := redisrepo.NewClient(cfg.RedisAddr(), cfg.RedisPassword, cfg.RedisDB, logger)
	if err != nil {
		logger.Warn("failed to connect to Redis, running without analytics cache", "error", err)
	} else {
		analyticsCache = redisrepo.NewAnalyticsCache(redisClient.GetClient(), logger)
		logger.Info("Redis analytics cache enabled")
	}

	// 6. Инициализируем слои приложения
	statsRepo := stats.NewStatsRepository(pool)
	statsService := service.NewStatsService(statsRepo, logger, analyticsCache)

	// 7. Создаём Kafka Consumer
	kafkaConsumer, err := kafkapkg.NewConsumer(kafkapkg.ConsumerConfig{
		Brokers: cfg.KafkaBrokers,
		Topic:   cfg.KafkaClicksTopic,
		GroupID: cfg.KafkaConsumerGroup,
		Handler: statsService.HandleClickEvent,
		Logger:  logger,
	})
	if err != nil {
		logger.Error("failed to create Kafka consumer", "error", err)
		os.Exit(1)
	}

	// Запускаем consumer
	kafkaConsumer.Start()

	logger.Info("Kafka consumer started",
		"brokers", cfg.KafkaBrokers,
		"topic", cfg.KafkaClicksTopic,
		"group_id", cfg.KafkaConsumerGroup,
	)

	// 8. Создаём HTTP handler
	statsHandler := handler.NewStatsHandler(statsService)

	// Health handler (только БД, без Redis)
	healthHandler := handler.NewHealthHandler(pool, nil)

	// 9. Настраиваем маршруты
	mux := http.NewServeMux()

	// Health-check и метрики
	mux.HandleFunc("GET /health", healthHandler.Health)
	mux.Handle("GET /metrics", promhttp.Handler())

	// Stats API endpoints
	mux.HandleFunc("GET /api/v1/links/{short}/stats", statsHandler.GetStats)
	mux.HandleFunc("GET /api/v1/stats/top", statsHandler.GetTopLinks)
	mux.HandleFunc("GET /api/v1/stats/trending", statsHandler.GetTrending)

	// 10. Применяем middleware
	handler := middleware.RequestID(middleware.Metrics(middleware.Logging(logger)(mux)))

	// 11. Создаём HTTP-сервер
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.ServerPort),
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 12. Запускаем сервер
	go func() {
		logger.Info("stats service started", "port", cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// 13. Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info("received shutdown signal", "signal", sig.String())
	logger.Info("shutting down stats service...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", "error", err)
	}

	// Останавливаем Kafka consumer
	if err := kafkaConsumer.Stop(); err != nil {
		logger.Error("failed to stop Kafka consumer", "error", err)
	}

	logger.Info("stats service stopped gracefully")
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
