package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Метрики для HTTP-запросов
var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// Метрики для кэша
	cacheHitsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"operation"},
	)

	cacheMissesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total number of cache misses",
		},
		[]string{"operation"},
	)

	cacheErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_errors_total",
			Help: "Total number of cache errors",
		},
		[]string{"operation"},
	)

	// Метрики для воркер-пула
	workerTasksProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "worker_tasks_processed_total",
			Help: "Total number of worker tasks processed",
		},
		[]string{"status"}, // success, error
	)

	workerErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "worker_errors_total",
			Help: "Total number of worker errors (failed tasks after retries)",
		},
	)

	workerProcessingDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "worker_processing_duration_seconds",
			Help:    "Worker task processing duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	// workerQueueSize — Gauge, устанавливается самим WorkerPool
	workerQueueSize prometheus.Gauge
)

// SetWorkerQueueSizeGauge регистрирует Gauge для размера очереди воркера
// (регистрируется один раз при создании WorkerPool)
func SetWorkerQueueSizeGauge(g prometheus.Gauge) {
	workerQueueSize = g
}

// RecordWorkerQueueSize обновляет текущий размер очереди
func RecordWorkerQueueSize(size int) {
	if workerQueueSize != nil {
		workerQueueSize.Set(float64(size))
	}
}

// metricsResponseWriter оборачивает ResponseWriter для перехвата status code
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newMetricsResponseWriter(w http.ResponseWriter) *metricsResponseWriter {
	return &metricsResponseWriter{w, http.StatusOK}
}

func (m *metricsResponseWriter) WriteHeader(code int) {
	m.statusCode = code
	m.ResponseWriter.WriteHeader(code)
}

// Metrics middleware собирает метрики для каждого HTTP-запроса
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Оборачиваем ResponseWriter
		rw := newMetricsResponseWriter(w)

		// Выполняем handler
		next.ServeHTTP(rw, r)

		// Вычисляем duration
		duration := time.Since(start).Seconds()

		// Извлекаем path (без query params)
		path := r.URL.Path

		// Инкрементируем счётчик запросов
		httpRequestsTotal.WithLabelValues(
			r.Method,
			path,
			strconv.Itoa(rw.statusCode),
		).Inc()

		// Записываем duration в гистограмму
		httpRequestDuration.WithLabelValues(
			r.Method,
			path,
		).Observe(duration)
	})
}

// RecordCacheHit записывает попадание в кэш
func RecordCacheHit(operation string) {
	cacheHitsTotal.WithLabelValues(operation).Inc()
}

// RecordCacheMiss записывает промах в кэше
func RecordCacheMiss(operation string) {
	cacheMissesTotal.WithLabelValues(operation).Inc()
}

// RecordCacheError записывает ошибку кэша
func RecordCacheError(operation string) {
	cacheErrorsTotal.WithLabelValues(operation).Inc()
}

// RecordWorkerTaskSuccess записывает успешную обработку задачи воркером
func RecordWorkerTaskSuccess() {
	workerTasksProcessedTotal.WithLabelValues("success").Inc()
}

// RecordWorkerTaskError записывает ошибку обработки задачи воркером
func RecordWorkerTaskError() {
	workerTasksProcessedTotal.WithLabelValues("error").Inc()
	workerErrorsTotal.Inc()
}

// RecordWorkerDuration записывает время обработки задачи воркером
func RecordWorkerDuration(seconds float64) {
	workerProcessingDuration.Observe(seconds)
}
