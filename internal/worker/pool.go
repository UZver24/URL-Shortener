package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/UZver24/URL-Shortener/internal/handler/middleware"
)

// ErrPoolClosed возвращается при попытке отправить задачу в закрытый пул
var ErrPoolClosed = errors.New("worker pool is closed")

// ErrQueueFull возвращается при переполнении буфера задач
var ErrQueueFull = errors.New("worker pool queue is full")

// TaskHandler — функция-обработчик задач
// Возвращает ошибку, если задачу не удалось обработать
type TaskHandler func(ctx context.Context, task Task) error

// WorkerPool — пул воркеров для асинхронной обработки задач
type WorkerPool struct {
	tasks      chan Task
	handler    TaskHandler
	workers    int
	wg         sync.WaitGroup
	quit       chan struct{}
	closed     bool
	closedMu   sync.Mutex
	logger     *slog.Logger
	maxRetries int           // максимальное число повторов (по умолчанию 5)
	baseDelay  time.Duration // базовая задержка для exponential backoff (по умолчанию 1s)
}

// WorkerPoolConfig — конфигурация WorkerPool
type WorkerPoolConfig struct {
	Workers    int           // количество воркеров
	BufferSize int           // размер буфера канала задач
	Handler    TaskHandler   // обработчик задач
	Logger     *slog.Logger  // логгер
	MaxRetries int           // максимальное число повторов (0 = 5)
	BaseDelay  time.Duration // базовая задержка для backoff (0 = 1s)
}

// NewWorkerPool создаёт новый пул воркеров
func NewWorkerPool(cfg WorkerPoolConfig) *WorkerPool {
	workers := cfg.Workers
	if workers <= 0 {
		workers = 5
	}
	bufferSize := cfg.BufferSize
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 5
	}
	baseDelay := cfg.BaseDelay
	if baseDelay <= 0 {
		baseDelay = 1 * time.Second
	}

	return &WorkerPool{
		tasks:      make(chan Task, bufferSize),
		handler:    cfg.Handler,
		workers:    workers,
		quit:       make(chan struct{}),
		logger:     cfg.Logger,
		maxRetries: maxRetries,
		baseDelay:  baseDelay,
	}
}

// Start запускает воркеры
func (p *WorkerPool) Start() {
	p.logger.Info("starting worker pool", "workers", p.workers, "buffer_size", cap(p.tasks))

	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// worker — один воркер, обрабатывающий задачи из канала
func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()

	p.logger.Debug("worker started", "worker_id", id)

	// Читаем задачи из канала, пока он не закрыт
	for task := range p.tasks {
		p.processTask(id, task)
	}

	p.logger.Debug("worker stopped", "worker_id", id)
}

// processTask обрабатывает одну задачу с exponential backoff
func (p *WorkerPool) processTask(workerID int, task Task) {
	start := time.Now()

	var lastErr error
	for attempt := 0; attempt < p.maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		err := p.handler(ctx, task)
		cancel()

		if err == nil {
			// Успех
			duration := time.Since(start).Seconds()
			middleware.RecordWorkerTaskSuccess()
			middleware.RecordWorkerDuration(duration)

			p.logger.Debug("task processed",
				"worker_id", workerID,
				"link_id", task.LinkID,
				"short_code", task.ShortCode,
				"duration_ms", duration*1000,
				"attempt", attempt+1,
			)
			return
		}

		lastErr = err
		p.logger.Warn("task failed, retrying",
			"worker_id", workerID,
			"link_id", task.LinkID,
			"short_code", task.ShortCode,
			"attempt", attempt+1,
			"error", err,
		)

		// Exponential backoff: 1s, 2s, 4s, 8s, 16s
		// Но только если это не последняя попытка
		if attempt < p.maxRetries-1 {
			delay := p.baseDelay * time.Duration(1<<uint(attempt))
			// Ограничим максимальную задержку 16 секундами
			if delay > 16*time.Second {
				delay = 16 * time.Second
			}

			select {
			case <-time.After(delay):
				// продолжаем
			case <-p.quit:
				// graceful shutdown — выходим
				return
			}
		}
	}

	// Все попытки провалились — логируем ошибку
	duration := time.Since(start).Seconds()
	middleware.RecordWorkerTaskError()
	middleware.RecordWorkerDuration(duration)

	p.logger.Error("task failed after all retries",
		"worker_id", workerID,
		"link_id", task.LinkID,
		"short_code", task.ShortCode,
		"retries", p.maxRetries,
		"error", lastErr,
	)
}

// Submit добавляет задачу в очередь
// Возвращает ErrPoolClosed если пул закрыт
// Возвращает ErrQueueFull если буфер переполнен
func (p *WorkerPool) Submit(task Task) error {
	p.closedMu.Lock()
	if p.closed {
		p.closedMu.Unlock()
		return ErrPoolClosed
	}
	p.closedMu.Unlock()

	// Non-blocking send — если буфер полон, возвращаем ошибку
	select {
	case p.tasks <- task:
		// Обновляем метрику размера очереди
		middleware.RecordWorkerQueueSize(len(p.tasks))
		return nil
	default:
		p.logger.Warn("worker pool queue is full, dropping task",
			"link_id", task.LinkID,
			"short_code", task.ShortCode,
		)
		return ErrQueueFull
	}
}

// Stop останавливает воркер-пул
// Гарантирует, что все задачи из буфера будут обработаны (включая retry)
func (p *WorkerPool) Stop() error {
	p.closedMu.Lock()
	if p.closed {
		p.closedMu.Unlock()
		return nil
	}
	p.closed = true
	p.closedMu.Unlock()

	p.logger.Info("stopping worker pool, waiting for tasks to complete",
		"pending_tasks", len(p.tasks),
	)

	// Закрываем канал задач — воркеры обработают оставшиеся и выйдут
	// НЕ закрываем quit — воркеры должны завершить текущие задачи (включая retry)
	close(p.tasks)

	// Ждём завершения всех воркеров
	p.wg.Wait()

	p.logger.Info("worker pool stopped")
	return nil
}

// QueueSize возвращает текущий размер очереди
func (p *WorkerPool) QueueSize() int {
	return len(p.tasks)
}

// BufferSize возвращает размер буфера канала задач
func (p *WorkerPool) BufferSize() int {
	return cap(p.tasks)
}

// String — строковое представление для логирования
func (p *WorkerPool) String() string {
	return fmt.Sprintf("WorkerPool{workers=%d, queue=%d/%d}",
		p.workers, len(p.tasks), cap(p.tasks))
}
