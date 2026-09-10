package worker

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// testLogger — logger для тестов
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
}

// TestPool_BasicProcessing — базовая обработка задач
func TestPool_BasicProcessing(t *testing.T) {
	var processed int32

	handler := func(ctx context.Context, task Task) error {
		atomic.AddInt32(&processed, 1)
		return nil
	}

	pool := NewWorkerPool(WorkerPoolConfig{
		Workers:    3,
		BufferSize: 10,
		Handler:    handler,
		Logger:     testLogger(),
	})
	pool.Start()

	// Отправляем 10 задач
	for i := 0; i < 10; i++ {
		err := pool.Submit(NewTask(int64(i), "test"))
		if err != nil {
			t.Fatalf("failed to submit task %d: %v", i, err)
		}
	}

	// Останавливаем пул (graceful — все задачи должны обработаться)
	if err := pool.Stop(); err != nil {
		t.Fatalf("failed to stop pool: %v", err)
	}

	// Проверяем, что все задачи обработаны
	if processed != 10 {
		t.Errorf("expected 10 tasks processed, got %d", processed)
	}
}

// TestPool_GracefulShutdown — graceful shutdown обрабатывает все задачи
func TestPool_GracefulShutdown(t *testing.T) {
	var processed int32
	const totalTasks = 100

	handler := func(ctx context.Context, task Task) error {
		time.Sleep(5 * time.Millisecond) // имитируем работу
		atomic.AddInt32(&processed, 1)
		return nil
	}

	pool := NewWorkerPool(WorkerPoolConfig{
		Workers:    5,
		BufferSize: 200,
		Handler:    handler,
		Logger:     testLogger(),
	})
	pool.Start()

	// Отправляем много задач
	for i := 0; i < totalTasks; i++ {
		err := pool.Submit(NewTask(int64(i), "test"))
		if err != nil {
			t.Fatalf("failed to submit task %d: %v", i, err)
		}
	}

	// Немедленно останавливаем — graceful shutdown должен дождаться всех
	if err := pool.Stop(); err != nil {
		t.Fatalf("failed to stop pool: %v", err)
	}

	// Все задачи должны быть обработаны
	if processed != totalTasks {
		t.Errorf("expected %d tasks processed, got %d", totalTasks, processed)
	}
}

// TestPool_QueueFull — переполнение буфера возвращает ErrQueueFull
func TestPool_QueueFull(t *testing.T) {
	blocker := make(chan struct{})

	handler := func(ctx context.Context, task Task) error {
		<-blocker // блокируем обработчик
		return nil
	}

	// Маленький буфер, чтобы быстро переполнить
	pool := NewWorkerPool(WorkerPoolConfig{
		Workers:    1,
		BufferSize: 2,
		Handler:    handler,
		Logger:     testLogger(),
	})
	pool.Start()

	// Отправляем первую задачу — воркер должен её взять и заблокироваться
	err := pool.Submit(NewTask(0, "test"))
	if err != nil {
		close(blocker)
		pool.Stop()
		t.Fatalf("failed to submit first task: %v", err)
	}

	// Даём время воркеру взять задачу
	time.Sleep(20 * time.Millisecond)

	// Заполняем буфер (2 задачи)
	for i := 1; i < 3; i++ {
		err := pool.Submit(NewTask(int64(i), "test"))
		if err != nil {
			close(blocker)
			pool.Stop()
			t.Fatalf("failed to submit task %d: %v", i, err)
		}
	}

	// Следующая задача должна вернуть ErrQueueFull
	err = pool.Submit(NewTask(99, "overflow"))

	// Освобождаем воркеры и останавливаем пул
	close(blocker)
	pool.Stop()

	if !errors.Is(err, ErrQueueFull) {
		t.Errorf("expected ErrQueueFull, got %v", err)
	}
}

// TestPool_SubmitAfterClose — отправка в закрытый пул возвращает ErrPoolClosed
func TestPool_SubmitAfterClose(t *testing.T) {
	handler := func(ctx context.Context, task Task) error {
		return nil
	}

	pool := NewWorkerPool(WorkerPoolConfig{
		Workers:    2,
		BufferSize: 10,
		Handler:    handler,
		Logger:     testLogger(),
	})
	pool.Start()
	pool.Stop()

	// Отправка в закрытый пул должна вернуть ErrPoolClosed
	err := pool.Submit(NewTask(1, "test"))
	if !errors.Is(err, ErrPoolClosed) {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

// TestPool_ExponentialBackoff — retry при ошибке с exponential backoff
func TestPool_ExponentialBackoff(t *testing.T) {
	var attempts int32

	handler := func(ctx context.Context, task Task) error {
		count := atomic.AddInt32(&attempts, 1)
		// Первые 3 попытки — ошибка, 4-я — успех
		if count < 4 {
			return errors.New("temporary error")
		}
		return nil
	}

	pool := NewWorkerPool(WorkerPoolConfig{
		Workers:    1,
		BufferSize: 10,
		Handler:    handler,
		Logger:     testLogger(),
		MaxRetries: 5,
		BaseDelay:  10 * time.Millisecond, // ускорим тест
	})
	pool.Start()

	err := pool.Submit(NewTask(1, "test"))
	if err != nil {
		t.Fatalf("failed to submit task: %v", err)
	}

	// Ждём завершения
	pool.Stop()

	// Должно быть 4 попытки (3 ошибки + 1 успех)
	if attempts != 4 {
		t.Errorf("expected 4 attempts (3 failures + 1 success), got %d", attempts)
	}
}

// TestPool_MaxRetriesExceeded — все попытки провалились
func TestPool_MaxRetriesExceeded(t *testing.T) {
	var attempts int32

	handler := func(ctx context.Context, task Task) error {
		atomic.AddInt32(&attempts, 1)
		return errors.New("permanent error")
	}

	pool := NewWorkerPool(WorkerPoolConfig{
		Workers:    1,
		BufferSize: 10,
		Handler:    handler,
		Logger:     testLogger(),
		MaxRetries: 3,
		BaseDelay:  10 * time.Millisecond, // ускорим тест
	})
	pool.Start()

	err := pool.Submit(NewTask(1, "test"))
	if err != nil {
		t.Fatalf("failed to submit task: %v", err)
	}

	// Ждём завершения
	pool.Stop()

	// Должно быть ровно 3 попытки
	if attempts != 3 {
		t.Errorf("expected 3 attempts (max retries), got %d", attempts)
	}
}

// TestPool_MultipleWorkers — параллельная обработка несколькими воркерами
func TestPool_MultipleWorkers(t *testing.T) {
	var processed int32
	const totalTasks = 50

	handler := func(ctx context.Context, task Task) error {
		time.Sleep(10 * time.Millisecond) // имитируем работу
		atomic.AddInt32(&processed, 1)
		return nil
	}

	pool := NewWorkerPool(WorkerPoolConfig{
		Workers:    5,
		BufferSize: 100,
		Handler:    handler,
		Logger:     testLogger(),
	})
	pool.Start()

	start := time.Now()

	for i := 0; i < totalTasks; i++ {
		pool.Submit(NewTask(int64(i), "test"))
	}

	pool.Stop()
	duration := time.Since(start)

	if processed != totalTasks {
		t.Errorf("expected %d tasks processed, got %d", totalTasks, processed)
	}

	// С 5 воркерами по 10ms на задачу и 50 задачами:
	// последовательно: 500ms, параллельно: ~100ms
	// Проверяем, что это реально быстрее чем последовательно
	if duration > 400*time.Millisecond {
		t.Errorf("expected parallel processing to be fast, got %v", duration)
	}
}

// TestPool_StopIdempotent — повторный вызов Stop безопасен
func TestPool_StopIdempotent(t *testing.T) {
	handler := func(ctx context.Context, task Task) error {
		return nil
	}

	pool := NewWorkerPool(WorkerPoolConfig{
		Workers:    2,
		BufferSize: 10,
		Handler:    handler,
		Logger:     testLogger(),
	})
	pool.Start()

	// Первый Stop
	if err := pool.Stop(); err != nil {
		t.Fatalf("first Stop failed: %v", err)
	}

	// Второй Stop (не должен паниковать)
	if err := pool.Stop(); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

// TestPool_QueueSize — проверка QueueSize
func TestPool_QueueSize(t *testing.T) {
	blocker := make(chan struct{})

	handler := func(ctx context.Context, task Task) error {
		<-blocker
		return nil
	}

	pool := NewWorkerPool(WorkerPoolConfig{
		Workers:    1,
		BufferSize: 10,
		Handler:    handler,
		Logger:     testLogger(),
	})
	pool.Start()

	// Отправляем 5 задач (1 в обработчике + 4 в буфере)
	for i := 0; i < 5; i++ {
		pool.Submit(NewTask(int64(i), "test"))
	}

	// Даём время обработчику взять первую задачу
	time.Sleep(10 * time.Millisecond)

	queueSize := pool.QueueSize()

	// Освобождаем воркеры и останавливаем пул
	close(blocker)
	pool.Stop()

	// В буфере должно быть около 4 задач (возможны race conditions)
	if queueSize < 3 || queueSize > 5 {
		t.Errorf("expected queue size 3-5, got %d", queueSize)
	}

	if pool.BufferSize() != 10 {
		t.Errorf("expected buffer size 10, got %d", pool.BufferSize())
	}
}

// TestTask_NewTask — проверка создания задачи
func TestTask_NewTask(t *testing.T) {
	before := time.Now()
	task := NewTask(42, "abc123")
	after := time.Now()

	if task.LinkID != 42 {
		t.Errorf("expected LinkID 42, got %d", task.LinkID)
	}
	if task.ShortCode != "abc123" {
		t.Errorf("expected ShortCode 'abc123', got '%s'", task.ShortCode)
	}
	if task.Timestamp.Before(before) || task.Timestamp.After(after) {
		t.Errorf("Timestamp %v is outside range [%v, %v]", task.Timestamp, before, after)
	}
}
