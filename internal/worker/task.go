package worker

import "time"

// Task представляет задачу для обработки воркером
type Task struct {
	LinkID    int64     // ID ссылки для обновления счётчика
	ShortCode string    // Короткий код (для логирования)
	Timestamp time.Time // Время создания задачи
}

// NewTask создаёт новую задачу
func NewTask(linkID int64, shortCode string) Task {
	return Task{
		LinkID:    linkID,
		ShortCode: shortCode,
		Timestamp: time.Now(),
	}
}
