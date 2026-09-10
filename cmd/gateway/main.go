package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/UZver24/URL-Shortener/internal/config"
	"github.com/UZver24/URL-Shortener/internal/handler/middleware"
)

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

	logger.Info("starting API gateway", "port", cfg.ServerPort)

	// 3. Создаём reverse proxy для Link Service
	linkServiceURL, err := url.Parse(cfg.LinkServiceURL)
	if err != nil {
		logger.Error("invalid LINK_SERVICE_URL", "error", err)
		os.Exit(1)
	}
	linkProxy := httputil.NewSingleHostReverseProxy(linkServiceURL)

	// 4. Создаём reverse proxy для Stats Service
	statsServiceURL, err := url.Parse(cfg.StatsServiceURL)
	if err != nil {
		logger.Error("invalid STATS_SERVICE_URL", "error", err)
		os.Exit(1)
	}
	statsProxy := httputil.NewSingleHostReverseProxy(statsServiceURL)

	// 5. Настраиваем маршруты
	mux := http.NewServeMux()

	// Health-check gateway
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"gateway"}`))
	})

	// Маршрутизация запросов
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Stats endpoints → Stats Service
		if strings.HasPrefix(path, "/api/v1/stats/") ||
			strings.HasSuffix(path, "/stats") {
			logger.Debug("routing to stats service", "path", path)
			statsProxy.ServeHTTP(w, r)
			return
		}

		// Все остальные запросы → Link Service
		logger.Debug("routing to link service", "path", path)
		linkProxy.ServeHTTP(w, r)
	})

	// 6. Применяем middleware
	handler := middleware.RequestID(middleware.Metrics(middleware.Logging(logger)(mux)))

	// 7. Создаём HTTP-сервер
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.ServerPort),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 8. Запускаем сервер
	go func() {
		logger.Info("API gateway started",
			"port", cfg.ServerPort,
			"link_service", cfg.LinkServiceURL,
			"stats_service", cfg.StatsServiceURL,
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// 9. Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info("received shutdown signal", "signal", sig.String())
	logger.Info("shutting down gateway...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", "error", err)
	}

	logger.Info("gateway stopped gracefully")
}
