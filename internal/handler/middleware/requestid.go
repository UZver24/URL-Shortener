package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

// RequestID добавляет уникальный идентификатор в каждый запрос
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Генерируем новый UUID для каждого запроса
		requestID := uuid.New().String()

		// Добавляем в контекст запроса
		ctx := context.WithValue(r.Context(), RequestIDKey, requestID)

		// Добавляем в заголовок ответа (чтобы клиент мог его увидеть)
		w.Header().Set("X-Request-ID", requestID)

		// Передаём дальше с обновлённым контекстом
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID извлекает request ID из контекста
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}
