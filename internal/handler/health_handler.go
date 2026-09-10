package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RedisPinger — интерфейс для проверки доступности Redis
type RedisPinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler обрабатывает health-check запросы
type HealthHandler struct {
	pool  *pgxpool.Pool
	redis RedisPinger // опциональный (может быть nil)
}

// NewHealthHandler создаёт новый health-check handler
// redis может быть nil — тогда кэш не проверяется
func NewHealthHandler(pool *pgxpool.Pool, redis RedisPinger) *HealthHandler {
	return &HealthHandler{pool: pool, redis: redis}
}

// HealthResponse — ответ health-check эндпоинта
type HealthResponse struct {
	Status   string `json:"status"`             // "ok" или "error"
	Database string `json:"database,omitempty"` // статус БД
	Cache    string `json:"cache,omitempty"`    // статус кэша
	Error    string `json:"error,omitempty"`    // причина ошибки (если есть)
}

// Health обрабатывает GET /health
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// Проверяем соединение с БД
	dbStatus := "ok"
	var errMsg string

	if err := h.pool.Ping(ctx); err != nil {
		dbStatus = "error"
		errMsg = err.Error()
	}

	// Проверяем соединение с Redis (если доступен)
	cacheStatus := ""
	if h.redis != nil {
		cacheStatus = "ok"
		if err := h.redis.Ping(ctx); err != nil {
			cacheStatus = "error"
			if errMsg == "" {
				errMsg = err.Error()
			}
		}
	}

	// Формируем ответ
	status := "ok"
	httpStatus := http.StatusOK

	if dbStatus != "ok" {
		// БД критична — если она не работает, возвращаем 503
		status = "error"
		httpStatus = http.StatusServiceUnavailable
	}

	resp := HealthResponse{
		Status:   status,
		Database: dbStatus,
		Cache:    cacheStatus,
		Error:    errMsg,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(resp)
}
