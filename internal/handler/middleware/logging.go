package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// responseRecorder оборачивает http.ResponseWriter для перехвата status code
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{w, http.StatusOK}
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// Logging логирует каждый HTTP-запрос
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Оборачиваем ResponseWriter для перехвата status code
			rec := newResponseRecorder(w)

			// Выполняем handler
			next.ServeHTTP(rec, r)

			// Вычисляем duration
			duration := time.Since(start)

			// Извлекаем request ID из контекста
			requestID := GetRequestID(r.Context())

			// Логируем запрос
			logger.Info("http request",
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.statusCode,
				"duration_ms", duration.Milliseconds(),
				"user_agent", r.UserAgent(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}
