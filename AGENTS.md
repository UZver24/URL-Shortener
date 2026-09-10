# AGENTS.md — правила для ИИ-ассистентов

> Этот файл — источник истины для всех ИИ-агентов, работающих с проектом.
> Перед началом работы обязательно прочитай его.

## О проекте

**URL Shortener** — учебный микросервисный сокращатель ссылок на Go + PostgreSQL.
Цель — подготовка к техническим собеседованиям в Ozon, Яндекс и другие крупные компании.

**Текущая фаза:** Фаза III — Микросервисы (Этап 5) ✅ ЗАВЕРШЕНА
**Завершено:** Фаза I — Монолит (тег `v1.0.0-monolith`), Фаза II — Кэш + Воркер-пул, Фаза III — Микросервисы + Kafka

## Ключевые файлы

| Файл | Назначение |
|------|-----------|
| `README.md` | Документация, Quick Start, API endpoints |
| `TODO.md` | План задач по этапам (детализация, чекбоксы) |
| `INTERVIEW.md` | Вопросы и ответы для собеседований по каждому этапу |
| `AGENTS.md` | Этот файл — правила для ИИ |

**Важно:** `INTERVIEW.md` содержит подробные вопросы и ответы для подготовки к собеседованиям. После каждого этапа обязательно добавляй туда новые вопросы по теме этапа.

## Команды

### Запуск (микросервисы)

```bash
# Запустить все микросервисы через Docker Compose
docker-compose up --build

# Запустить только инфраструк (PostgreSQL + Redis + Kafka)
docker-compose up -d postgres redis kafka

# Запустить Link Service локально
SERVICE_NAME=link-service go run ./cmd/link-service

# Запустить Stats Service локально
SERVICE_NAME=stats-service go run ./cmd/stats-service

# Запустить Gateway локально
SERVICE_NAME=gateway go run ./cmd/gateway

# Запустить монолит (обратная совместимость)
go run ./cmd/api
```

### Тестирование

```bash
go test -v ./...
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

### Линтеры

```bash
gofmt -l .           # Форматирование
go vet ./...         # Стандартные проверки
staticcheck ./...    # Углублённый анализ
```

### API (curl примеры)

```bash
curl -X POST http://localhost:8080/api/v1/links -H "Content-Type: application/json" -d '{"url": "https://example.com"}'
curl -v http://localhost:8080/<short_code>
curl http://localhost:8080/api/v1/links/<short_code>/stats
curl -X DELETE http://localhost:8080/api/v1/links/<short_code>
curl http://localhost:8080/health
curl http://localhost:8080/metrics
```

## Структура проекта

```
URL-Shortener/
├── cmd/
│   ├── api/                             # Монолит (Фаза I-II, обратная совместимость)
│   │   ├── main.go
│   │   └── migrations/*.sql
│   ├── link-service/                    # Link Service (CRUD + redirect)
│   │   ├── main.go
│   │   └── migrations/*.sql
│   ├── stats-service/                   # Stats Service (Kafka consumer + аналитика)
│   │   ├── main.go
│   │   └── migrations/*.sql
│   └── gateway/                         # API Gateway (reverse proxy)
│       └── main.go
├── internal/
│   ├── config/                          # Загрузка конфигурации из env
│   ├── handler/                         # HTTP-обработчики (внешний слой)
│   │   ├── link_handler.go              # Link Service handlers
│   │   ├── stats_handler.go             # Stats Service handlers
│   │   ├── health_handler.go            # Health-check
│   │   └── middleware/                  # RequestID, Logging, Metrics
│   ├── messaging/                       # Messaging
│   │   └── kafka/                       # Kafka producer/consumer
│   │       ├── events.go               # ClickEvent + сериализация
│   │       ├── producer.go             # Publisher
│   │       └── consumer.go             # Consumer с retry
│   ├── service/                         # Бизнес-логика (средний слой)
│   │   ├── link_service.go             # Link Service
│   │   └── stats_service.go            # Stats Service
│   ├── repository/                      # Работа с данными (внутренний слой)
│   │   ├── postgres/                   # PostgreSQL
│   │   │   ├── link_repo.go            # Links CRUD
│   │   │   ├── postgres.go             # Connection pool
│   │   │   └── stats/                  # Stats repository
│   │   │       └── stats_repo.go
│   │   └── redis/                      # Redis cache
│   ├── model/                          # Доменные типы и ошибки
│   └── worker/                         # Воркер-пул (монолит, обратная совместимость)
├── docker-compose.yml                  # PostgreSQL + Redis + Kafka + 3 сервиса
├── Dockerfile.link-service             # Dockerfile для Link Service
├── Dockerfile.stats-service            # Dockerfile для Stats Service
├── Dockerfile.gateway                  # Dockerfile для Gateway
├── Dockerfile                          # Dockerfile для монолита
├── .env.example                        # Шаблон переменных окружения
└── go.mod
```

## Архитектурные правила

### 1. Слоистая архитектура (Clean Architecture)

```
Handler (HTTP) → Service (бизнес-логика) → Repository (данные)
```

- **Handler**: только HTTP-контекст (парсинг JSON, статусы, ответы). Никакой бизнес-логики.
- **Service**: бизнес-правила, валидация, генерация кодов. Не знает про HTTP и SQL.
- **Repository**: работа с БД/кэшем. Только SQL-запросы и CRUD.

**Зависимости направлены внутрь:** `handler → service → repository`.

### 2. Dependency Injection через интерфейсы

- Service принимает Repository через **интерфейс**, а не конкретный тип
- Это позволяет мокировать зависимости в тестах
- Пример:
  ```go
  type LinkRepository interface {
      Create(ctx context.Context, link *model.Link) error
      GetByCode(ctx context.Context, code string) (*model.Link, error)
      // ...
  }
  
  type LinkService struct {
      repo   LinkRepository  // интерфейс, не *postgres.LinkRepository
      logger *slog.Logger
  }
  ```

### 3. Обработка ошибок

- Используй **`errors.Is(err, target)`**, а не `err == target`
- Оборачивай ошибки с контекстом: `fmt.Errorf("failed to create link: %w", err)`
- Кастомные ошибки определены в `internal/model/errors.go`
- Handler маппит ошибки на HTTP-коды через `handleServiceError()`

### 4. Логирование через `slog`

- Используй `*slog.Logger`, передаваемый через конструктор (DI)
- НЕ используй `log.Println` или `slog.Default()`
- Уровни:
  - `Info` — штатные события (link created, server started)
  - `Warn` — нештатные, но не критичные (invalid URL, collision)
  - `Error` — критичные ошибки (failed to connect to DB)

### 5. Контекст

- `context.Context` всегда первый параметр функции
- Используй `r.Context()` в handlers, `context.Background()` для фоновых задач

### 6. Конфигурация через env

- Все настройки — в переменных окружения (см. `.env.example`)
- Шаблон `.env.example` коммитится, `.env` — в `.gitignore`
- Загрузка через `github.com/joho/godotenv` + `internal/config/`

## Стиль кода

- Форматирование: `gofmt` (обязательно)
- Именование файлов: `snake_case.go`
- Именование пакетов: короткие, в единственном числе (`handler`, `service`, `postgres`)
- Комментарии к экспортируемым типам и функциям — на русском или английском
- Длина кода: предпочитай ясность над краткостью

## Требования к тестам

| Слой | Целевое покрытие | Тип тестов |
|------|------------------|-----------|
| `service` | ≥ 70% | Unit-тесты с мок-репозиторием |
| `handler` | ≥ 80% | Integration-тесты через `httptest.NewRecorder` |
| `repository` | — | Integration-тесты с реальной БД (testcontainers) |

- Моки пишем вручную (без `gomock`) — это полезно для собеседований
- Каждый тест должен быть изолирован (своя БД/мок)

## Принципы работы (из README.md)

1. **Итеративность** — не переходим к следующему этапу, пока не работает текущий
2. **Объяснение каждого шага** — после каждого блока кода объяснение
3. **Практики из реальной разработки** — структура как в продакшене
4. **Собеседовательный фокус** — в конце каждого этапа блок вопросов/ответов в `INTERVIEW.md`
5. **Обоснование решений** — почему именно так, а не иначе

## Формат ответа при работе над этапом

При реализации нового шага придерживайся структуры:

1. **Что делаем** — цель шага
2. **Команда** — bash-команда (если применимо)
3. **Код** — реализация с комментариями
4. **Зачем это нужно** — объяснение выбора и альтернатив
5. **Как проверить** — команды для верификации

## Текущий план (Фаза II — ЗАВЕРШЕНА)

**Этап 3. Кэш (Redis)** — ✅ выполнен:
- Redis в docker-compose
- Клиент `github.com/redis/go-redis/v9`
- Cache-aside паттерн
- Graceful degradation
- Метрики hit/miss/errors

**Этап 4. Конкурентность** — ✅ выполнен:
- Воркер-пул для асинхронной записи счётчиков
- Каналы + `sync.WaitGroup`
- Graceful shutdown воркеров
- Exponential backoff при ошибках
- Метрики воркер-пула

## Следующий план (Фаза III — Оптимизация)

**Этап 6. Оптимизация (алгоритмы)** — следующий:
- HyperLogLog для уникальных переходов
- Sorted Sets для топа ссылок
- Bloom Filter для проверки существования
- Base62 vs Snowflake ID
- Нагрузочное тестирование

## Чего НЕ делать

- ❌ Не добавляй новые зависимости без обоснования
- ❌ Не меняй архитектуру слоёв без согласования
- ❌ Не используй ORM (мы учимся писать SQL вручную)
- ❌ Не используй фреймворки (chi, gin, echo) — стандартная библиотека `net/http`
- ❌ Не коммить `.env` и бинарники (они в `.gitignore`)
- ❌ Не пропускай собеседовательные блоки — это ключевая часть проекта

## Что можно делать

- ✅ Предлагать улучшения (но сначала объясни почему)
- ✅ Выполнять задачи из других этапов, если это целесообразно
- ✅ Рефакторить код, если находишь проблемы
- ✅ Добавлять тесты для существующего кода
- ✅ Коммитить может только пользователь (ИИ не коммитит)

---

**Версия:** 1.0
**Дата:** после завершения Фазы I
**Статус:** актуально для Фазы II
