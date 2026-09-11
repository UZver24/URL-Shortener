package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config хранит все настройки приложения
type Config struct {
	// PostgreSQL
	PostgresHost     string
	PostgresPort     int
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string

	// Redis
	RedisHost     string
	RedisPort     int
	RedisPassword string
	RedisDB       int
	RedisTTL      int // TTL в секундах

	// Kafka
	KafkaBrokers       []string // Список брокеров Kafka
	KafkaClicksTopic   string   // Топик для событий кликов
	KafkaConsumerGroup string   // Consumer Group ID

	// Worker Pool (для монолита, обратная совместимость)
	WorkerCount      int // количество воркеров
	WorkerBufferSize int // размер буфера канала задач

	// Server
	ServerPort  int
	ServiceName string // Имя сервиса (link-service, stats-service, gateway)

	// Gateway (только для API Gateway)
	LinkServiceURL  string // URL Link Service
	StatsServiceURL string // URL Stats Service
}

// Load загружает конфигурацию из переменных окружения
func Load() (*Config, error) {
	// Загружаем .env файл (если существует)
	// Если файла нет — godotenv вернёт ошибку, но мы её игнорируем
	// (переменные могут быть установлены через docker-compose или shell)
	_ = godotenv.Load()

	cfg := &Config{}

	// PostgreSQL
	cfg.PostgresHost = getEnv("POSTGRES_HOST", "localhost")
	cfg.PostgresPort = getEnvAsInt("POSTGRES_PORT", 5432)
	cfg.PostgresUser = getEnv("POSTGRES_USER", "shortener")
	cfg.PostgresPassword = getEnv("POSTGRES_PASSWORD", "secret")
	cfg.PostgresDB = getEnv("POSTGRES_DB", "shortener")

	// Redis
	cfg.RedisHost = getEnv("REDIS_HOST", "localhost")
	cfg.RedisPort = getEnvAsInt("REDIS_PORT", 6379)
	cfg.RedisPassword = getEnv("REDIS_PASSWORD", "")
	cfg.RedisDB = getEnvAsInt("REDIS_DB", 0)
	cfg.RedisTTL = getEnvAsInt("REDIS_TTL", 3600) // 1 час по умолчанию

	// Kafka
	cfg.KafkaBrokers = getEnvAsSlice("KAFKA_BROKERS", []string{"localhost:9094"})
	cfg.KafkaClicksTopic = getEnv("KAFKA_CLICKS_TOPIC", "link.clicks")
	cfg.KafkaConsumerGroup = getEnv("KAFKA_CONSUMER_GROUP", "stats-service")

	// Worker Pool
	cfg.WorkerCount = getEnvAsInt("WORKER_COUNT", 5)
	cfg.WorkerBufferSize = getEnvAsInt("WORKER_BUFFER_SIZE", 1000)

	// Server
	cfg.ServerPort = getEnvAsInt("SERVER_PORT", 8080)
	cfg.ServiceName = getEnv("SERVICE_NAME", "url-shortener")

	// Gateway
	cfg.LinkServiceURL = getEnv("LINK_SERVICE_URL", "http://localhost:8081")
	cfg.StatsServiceURL = getEnv("STATS_SERVICE_URL", "http://localhost:8082")

	// Валидация обязательных полей
	if cfg.PostgresPassword == "" {
		return nil, fmt.Errorf("POSTGRES_PASSWORD is required")
	}

	return cfg, nil
}

// DatabaseURL возвращает строку подключения к PostgreSQL
// Формат: postgres://user:password@host:port/dbname?sslmode=disable
func (c *Config) DatabaseURL() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.PostgresUser,
		c.PostgresPassword,
		c.PostgresHost,
		c.PostgresPort,
		c.PostgresDB,
	)
}

// RedisAddr возвращает адрес Redis в формате host:port
func (c *Config) RedisAddr() string {
	return fmt.Sprintf("%s:%d", c.RedisHost, c.RedisPort)
}

// IsLinkService проверяет, является ли текущий сервис Link Service
func (c *Config) IsLinkService() bool {
	return c.ServiceName == "link-service"
}

// IsStatsService проверяет, является ли текущий сервис Stats Service
func (c *Config) IsStatsService() bool {
	return c.ServiceName == "stats-service"
}

// IsGateway проверяет, является ли текущий сервис Gateway
func (c *Config) IsGateway() bool {
	return c.ServiceName == "gateway"
}

// getEnv возвращает значение переменной окружения или значение по умолчанию
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// getEnvAsInt возвращает значение переменной окружения как int или значение по умолчанию
func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

// getEnvAsSlice возвращает значение переменной окружения как слайс строк (через запятую)
func getEnvAsSlice(key string, defaultValue []string) []string {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}

	parts := strings.Split(valueStr, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return defaultValue
	}

	return result
}
