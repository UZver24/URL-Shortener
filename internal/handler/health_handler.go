package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthHandler обрабатывает health-check запросы
type HealthHandler struct {
	pool *pgxpool.Pool
}

// NewHealthHandler создаёт новый health-check handler
func NewHealthHandler(pool *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{pool: pool}
}

// HealthResponse — ответ health-check эндпоинта
type HealthResponse struct {
	Status   string `json:"status"`             // "ok" или "error"
	Database string `json:"database,omitempty"` // статус БД
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

	// Формируем ответ
	status := "ok"
	httpStatus := http.StatusOK

	if dbStatus != "ok" {
		status = "error"
		httpStatus = http.StatusServiceUnavailable
	}

	resp := HealthResponse{
		Status:   status,
		Database: dbStatus,
		Error:    errMsg,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(resp)
}
