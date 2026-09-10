package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/UZver24/URL-Shortener/internal/model"
	"github.com/UZver24/URL-Shortener/internal/service"
)

// LinkHandler обрабатывает HTTP-запросы для ссылок
type LinkHandler struct {
	linkService *service.LinkService
}

// NewLinkHandler создаёт новый обработчик
func NewLinkHandler(linkService *service.LinkService) *LinkHandler {
	return &LinkHandler{linkService: linkService}
}

// CreateLink обрабатывает POST /api/v1/links
func (h *LinkHandler) CreateLink(w http.ResponseWriter, r *http.Request) {
	var req model.CreateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	link, err := h.linkService.CreateLink(r.Context(), req.URL, req.CustomCode)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	resp := model.CreateLinkResponse{
		Short:    link.ShortCode,
		Original: link.OriginalURL,
	}

	respondJSON(w, http.StatusCreated, resp)
}

// Redirect обрабатывает GET /{short}
func (h *LinkHandler) Redirect(w http.ResponseWriter, r *http.Request) {
	// Извлекаем short_code из URL (убираем leading "/")
	code := strings.TrimPrefix(r.URL.Path, "/")
	if code == "" {
		respondError(w, http.StatusBadRequest, "Short code is required")
		return
	}

	// Извлекаем User-Agent и Referer для аналитики
	userAgent := r.UserAgent()
	referer := r.Referer()

	originalURL, err := h.linkService.GetOriginalURL(r.Context(), code, userAgent, referer)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// HTTP 302 Found (временный редирект)
	http.Redirect(w, r, originalURL, http.StatusFound)
}

// GetStats обрабатывает GET /api/v1/links/{short}/stats
func (h *LinkHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	// Извлекаем short_code из URL
	// URL: /api/v1/links/{short}/stats
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		respondError(w, http.StatusBadRequest, "Invalid path")
		return
	}
	code := parts[4] // [0]="", [1]="api", [2]="v1", [3]="links", [4]="{short}", [5]="stats"

	link, err := h.linkService.GetStats(r.Context(), code)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	resp := model.LinkStatsResponse{
		Short:          link.ShortCode,
		Original:       link.OriginalURL,
		Clicks:         link.Clicks,
		CreatedAt:      link.CreatedAt,
		LastAccessedAt: link.LastAccessedAt,
	}

	respondJSON(w, http.StatusOK, resp)
}

// DeleteLink обрабатывает DELETE /api/v1/links/{short}
func (h *LinkHandler) DeleteLink(w http.ResponseWriter, r *http.Request) {
	// Извлекаем short_code из URL
	// URL: /api/v1/links/{short}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		respondError(w, http.StatusBadRequest, "Invalid path")
		return
	}
	code := parts[4]

	err := h.linkService.DeleteLink(r.Context(), code)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// respondJSON отправляет JSON-ответ
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError отправляет JSON-ответ с ошибкой
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

// handleServiceError маппит ошибки сервиса на HTTP-коды
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, model.ErrLinkNotFound):
		respondError(w, http.StatusNotFound, "Link not found")
	case errors.Is(err, model.ErrInvalidInput):
		respondError(w, http.StatusBadRequest, "Invalid input")
	case errors.Is(err, model.ErrLinkAlreadyExists):
		respondError(w, http.StatusConflict, "Link already exists")
	case errors.Is(err, model.ErrShortCodeAlreadyTaken):
		respondError(w, http.StatusConflict, "Short code already taken")
	default:
		respondError(w, http.StatusInternalServerError, "Internal server error")
	}
}
