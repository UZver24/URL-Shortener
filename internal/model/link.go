package model

import "time"

// Link представляет сокращённую ссылку в системе
type Link struct {
	ID             int64      // Уникальный идентификатор
	ShortCode      string     // Короткий код (например, "abc123")
	OriginalURL    string     // Оригинальный URL
	CreatedAt      time.Time  // Дата создания
	Clicks         int64      // Количество переходов
	LastAccessedAt *time.Time // Дата последнего перехода (может быть nil)
}

// CreateLinkRequest — запрос на создание ссылки
type CreateLinkRequest struct {
	URL        string `json:"url"`
	CustomCode string `json:"custom_code,omitempty"` // Опционально: пользовательский код
}

// CreateLinkResponse — ответ после создания ссылки
type CreateLinkResponse struct {
	Short    string `json:"short"`
	Original string `json:"original"`
}

// LinkStatsResponse — ответ со статистикой
type LinkStatsResponse struct {
	Short          string     `json:"short"`
	Original       string     `json:"original"`
	Clicks         int64      `json:"clicks"`
	CreatedAt      time.Time  `json:"created_at"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
}
