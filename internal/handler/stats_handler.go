package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/UZver24/URL-Shortener/internal/service"
)

// StatsHandler обрабатывает HTTP-запросы для статистики
type StatsHandler struct {
	statsService *service.StatsService
}

// NewStatsHandler создаёт новый обработчик статистики
func NewStatsHandler(statsService *service.StatsService) *StatsHandler {
	return &StatsHandler{statsService: statsService}
}

// GetStats обрабатывает GET /api/v1/links/{short}/stats
func (h *StatsHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	// Извлекаем short_code из URL
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		respondStatsError(w, http.StatusBadRequest, "Invalid path")
		return
	}
	code := parts[4]

	statsData, err := h.statsService.GetStats(r.Context(), code)
	if err != nil {
		respondStatsError(w, http.StatusNotFound, "Stats not found")
		return
	}

	resp := map[string]interface{}{
		"short":            statsData.ShortCode,
		"original":         statsData.OriginalURL,
		"total_clicks":     statsData.TotalClicks,
		"unique_clicks":    statsData.UniqueClicks,
		"last_clicked_at":  statsData.LastClickedAt,
		"created_at":       statsData.CreatedAt,
		"updated_at":       statsData.UpdatedAt,
	}

	respondStatsJSON(w, http.StatusOK, resp)
}

// GetTopLinks обрабатывает GET /api/v1/stats/top?limit=10
func (h *StatsHandler) GetTopLinks(w http.ResponseWriter, r *http.Request) {
	// Извлекаем limit из query params
	limitStr := r.URL.Query().Get("limit")
	limit := 10 // по умолчанию

	if limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err == nil && parsed > 0 {
			limit = parsed
		}
	}

	links, err := h.statsService.GetTopLinks(r.Context(), limit)
	if err != nil {
		respondStatsError(w, http.StatusInternalServerError, "Failed to get top links")
		return
	}

	respondStatsJSON(w, http.StatusOK, map[string]interface{}{
		"links": links,
		"count": len(links),
	})
}

// GetTrending обрабатывает GET /api/v1/stats/trending?limit=10
func (h *StatsHandler) GetTrending(w http.ResponseWriter, r *http.Request) {
	// Извлекаем limit из query params
	limitStr := r.URL.Query().Get("limit")
	limit := 10 // по умолчанию

	if limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err == nil && parsed > 0 {
			limit = parsed
		}
	}

	links, err := h.statsService.GetTrending(r.Context(), limit)
	if err != nil {
		respondStatsError(w, http.StatusInternalServerError, "Failed to get trending links")
		return
	}

	respondStatsJSON(w, http.StatusOK, map[string]interface{}{
		"links": links,
		"count": len(links),
	})
}

// respondStatsJSON отправляет JSON-ответ
func respondStatsJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondStatsError отправляет JSON-ответ с ошибкой
func respondStatsError(w http.ResponseWriter, status int, message string) {
	respondStatsJSON(w, status, map[string]string{"error": message})
}
