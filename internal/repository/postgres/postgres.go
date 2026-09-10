package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool создаёт пул соединений с PostgreSQL
func NewPool(databaseURL string) (*pgxpool.Pool, error) {
	// Парсим конфигурацию пула из URL
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Настраиваем параметры пула
	config.MaxConns = 10                      // Максимум 10 соединений
	config.MinConns = 2                       // Минимум 2 соединения (держим "тёплыми")
	config.MaxConnLifetime = time.Hour        // Максимальное время жизни соединения
	config.MaxConnIdleTime = 30 * time.Minute // Время простоя перед закрытием

	// Создаём пул
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// Проверяем соединение
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}
