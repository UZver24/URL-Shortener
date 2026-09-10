# URL Shortener — учебный проект на Go

## Цель проекта

Разработать микросервисный сокращатель ссылок (URL Shortener) на языке Go с использованием PostgreSQL. Проект учебный: главное — не скорость разработки, а понимание каждого шага и применение практик, которые спрашивают на технических собеседованиях в Ozon, Яндекс и других крупных компаниях.

## Быстрый старт

### Требования

- Go 1.22+
- Docker и Docker Compose
- curl (для тестирования API)

### 1. Клонировать репозиторий

```bash
git clone https://github.com/UZver24/URL-Shortener.git
cd URL-Shortener
```

### 2. Настроить конфигурацию

```bash
cp .env.example .env
# Отредактировать .env при необходимости (по умолчанию всё работает)
```

### 3. Запустить PostgreSQL

```bash
docker-compose up -d
```

Проверить, что БД запустилась:

```bash
docker-compose ps
```

### 4. Запустить приложение

**Вариант A: через `go run` (для разработки)**

```bash
go run ./cmd/api
```

**Вариант B: собрать бинарник и запустить**

```bash
go build -o url-shortener ./cmd/api
./url-shortener
```

Приложение запустится на `http://localhost:8080`.

### 5. Проверить работу API

**Создать короткую ссылку:**

```bash
curl -X POST http://localhost:8080/api/v1/links \
  -H "Content-Type: application/json" \
  -d '{"url": "https://github.com"}'
```

Ответ:

```json
{"short":"abc123","original":"https://github.com"}
```

**Перейти по короткой ссылке (редирект):**

```bash
curl -v http://localhost:8080/abc123
# HTTP/1.1 302 Found
# Location: https://github.com
```

**Получить статистику:**

```bash
curl http://localhost:8080/api/v1/links/abc123/stats
```

Ответ:

```json
{
  "short": "abc123",
  "original": "https://github.com",
  "clicks": 1,
  "created_at": "2026-09-10T20:51:53+03:00",
  "last_accessed_at": "2026-09-10T20:52:17+03:00"
}
```

**Удалить ссылку:**

```bash
curl -X DELETE http://localhost:8080/api/v1/links/abc123
# HTTP 204 No Content
```

### 6. Запустить тесты

```bash
go test -v ./...
```

Ожидаемый результат:

```
ok  	github.com/UZver24/URL-Shortener/internal/handler	(coverage: 88.5%)
ok  	github.com/UZver24/URL-Shortener/internal/service	(coverage: 84.3%)
```

### 7. Остановить приложение и БД

```bash
# Остановить приложение: Ctrl+C в терминале с go run

# Остановить PostgreSQL
docker-compose down

# Остановить PostgreSQL и удалить данные
docker-compose down -v
```

### Альтернатива: Запуск всего через Docker Compose

```bash
# Собрать и запустить всё (PostgreSQL + API)
docker-compose up --build

# Остановить
docker-compose down

# Остановить и удалить данные
docker-compose down -v
```

Приложение будет доступно на `http://localhost:8080`.

---

## API Endpoints

| Метод | Путь | Описание | Коды ответа |
|-------|------|----------|-------------|
| `GET` | `/health` | Health-check (readiness probe) | 200, 503 |
| `GET` | `/metrics` | Prometheus-метрики | 200 |
| `POST` | `/api/v1/links` | Создать короткую ссылку | 201, 400, 409 |
| `GET` | `/{short}` | Редирект на оригинальный URL | 302, 404 |
| `GET` | `/api/v1/links/{short}/stats` | Получить статистику | 200, 404 |
| `DELETE` | `/api/v1/links/{short}` | Удалить ссылку | 204, 404 |

### Health-check и метрики

**Проверить здоровье приложения:**

```bash
curl http://localhost:8080/health
```

Ответ:

```json
{"status": "ok", "database": "ok"}
```

**Получить Prometheus-метрики:**

```bash
curl http://localhost:8080/metrics
```

Собираемые метрики:
- `http_requests_total` — количество HTTP-запросов (method, path, status)
- `http_request_duration_seconds` — время обработки запроса (гистограмма)
- `db_pool_active_connections` — активные соединения с БД
- `db_pool_idle_connections` — свободные соединения с БД
- `db_pool_total_connections` — всего соединений в пуле
- `cache_hits_total` — количество попаданий в кэш (operation)
- `cache_misses_total` — количество промахов в кэше (operation)
- `cache_errors_total` — количество ошибок кэша (operation)
- `worker_queue_size` — текущий размер очереди воркер-пула
- `worker_tasks_processed_total` — количество обработанных задач (status)
- `worker_errors_total` — количество ошибок обработки задач
- `worker_processing_duration_seconds` — время обработки задачи воркером

### Примеры запросов

**Создание ссылки с custom_code:**

```bash
curl -X POST http://localhost:8080/api/v1/links \
  -H "Content-Type: application/json" \
  -d '{"url": "https://google.com", "custom_code": "google"}'
```

**Обработка ошибок:**

```bash
# Невалидный URL
curl -X POST http://localhost:8080/api/v1/links \
  -H "Content-Type: application/json" \
  -d '{"url": "not-a-url"}'
# 400 Bad Request: {"error": "Invalid input"}

# Несуществующая ссылка
curl http://localhost:8080/nonexistent
# 404 Not Found: {"error": "Link not found"}
```

---

## Технологический стек

- **Язык:** Go (версия 1.22+)
- **СУБД:** PostgreSQL 16+
- **Кэш:** Redis 7+
- **HTTP-сервер:** стандартная библиотека `net/http` (без фреймворков на первом этапе)
- **Драйвер БД:** `github.com/jackc/pgx/v5` (или `database/sql` + `lib/pq`)
- **Redis клиент:** `github.com/redis/go-redis/v9`
- **Конфигурация:** переменные окружения + `github.com/joho/godotenv`
- **Логирование:** `log/slog` (стандартная библиотека)
- **Контейнеризация:** Docker + docker-compose
- **Миграции:** `golang-migrate/migrate` или ручные SQL-скрипты
- **Тестирование:** стандартный `testing` + `httptest`
- **Очереди (этап 5):** Apache Kafka (KRaft mode, `github.com/segmentio/kafka-go`) ✅

## Принципы работы над проектом

- **Итеративность.** Проект разбит на этапы. Каждый этап — законченный, работающий кусок. Не переходим к следующему, пока не работает текущий.
- **Объяснение каждого шага.** После каждого блока кода — объяснение: что сделано, почему именно так, какие есть альтернативы, какие компромиссы.
- **Практики из реальной разработки.** Не «игрушечный код», а структура, максимально приближенная к продакшену: разделение на слои, обработка ошибок, конфигурация через env, graceful shutdown.
- **Собеседовательный фокус.** В конце каждого этапа — блок «Что могут спросить на собеседовании по этой теме» с возможными вопросами и вариантами ответов.
- **Архитектурные решения с обоснованием.** Почему PostgreSQL, а не MongoDB? Почему REST, а не gRPC на этом этапе? Почему именно такая структура пакетов?

## Функциональные требования

### Базовый функционал (MVP)

- `POST /api/v1/links` — создать короткую ссылку.
  - Входные данные (JSON): `{"url": "https://example.com/very/long/path"}`
  - Выход: `{"short": "abc123", "original": "https://example.com/very/long/path"}`
- `GET /{short}` — редирект на оригинальный URL (HTTP 301 или 302).
- `GET /api/v1/links/{short}/stats` — статистика по ссылке (количество переходов, дата создания, последний переход).
- `DELETE /api/v1/links/{short}` — удалить ссылку.

### Расширенный функционал (после MVP)

- **Срок жизни ссылки (TTL).** Если указан `expires_at`, по истечении срока ссылка перестаёт работать.
- **Кастомные короткие ссылки.** Пользователь может предложить свой код: `{"url": "...", "custom_code": "my-link"}`.
- **Асинхронная запись статистики.** Переходы записываются в очередь, отдельный воркер их агрегирует (уменьшает latency ответа).

## Архитектура

> Нумерация ниже — **фазы эволюции архитектуры** (I, II, III). Это не то же самое, что этапы в «Плане этапов» (0–7); фазы показывают, как растёт система в целом, а этапы — конкретные шаги реализации.

### Фаза I — монолит

```
Client → HTTP API (Go) → PostgreSQL
```

### Фаза II — добавляем кэш

```
Client → HTTP API (Go) → Redis (кэш) → PostgreSQL
```

### Фаза III — микросервисы ✅

```
                       ┌──────────────────┐
 Client → Gateway ────→│  Link Service    │ → PostgreSQL (links) → Redis
                       └────────┬─────────┘
                                │ (ClickEvent)
                                ▼
                       ┌──────────────────┐
                       │      Kafka       │
                       └────────┬─────────┘
                                ▼
                       ┌──────────────────┐
                       │  Stats Service   │ → PostgreSQL (stats)
                       └──────────────────┘
```

**Порты:**
- API Gateway: `8080` (единая точка входа)
- Link Service: `8081` (CRUD + redirect)
- Stats Service: `8082` (аналитика)

На каждой фазе — обсуждение: почему переходим, какие проблемы решаем, какие появляются новые.

## Структура проекта (актуальная, Фаза III)

```
url-shortener/
├── cmd/
│   ├── api/                         # Монолит (Фаза I-II, обратная совместимость)
│   │   ├── main.go
│   │   └── migrations/*.sql
│   ├── link-service/                # Link Service (CRUD + redirect + Kafka producer)
│   │   ├── main.go
│   │   └── migrations/*.sql
│   ├── stats-service/               # Stats Service (Kafka consumer + аналитика)
│   │   ├── main.go
│   │   └── migrations/*.sql
│   └── gateway/                     # API Gateway (reverse proxy)
│       └── main.go
├── internal/
│   ├── config/                      # Конфигурация из env
│   ├── handler/                     # HTTP-обработчики
│   │   ├── link_handler.go          # Link Service handlers
│   │   ├── stats_handler.go         # Stats Service handlers
│   │   ├── health_handler.go        # Health-check
│   │   └── middleware/              # RequestID, Logging, Metrics
│   ├── messaging/
│   │   └── kafka/                   # Kafka producer/consumer
│   │       ├── events.go            # ClickEvent + сериализация
│   │       ├── producer.go          # Publisher
│   │       └── consumer.go          # Consumer с retry
│   ├── service/                     # Бизнес-логика
│   │   ├── link_service.go          # Link Service
│   │   └── stats_service.go         # Stats Service
│   ├── repository/
│   │   ├── postgres/                # PostgreSQL
│   │   │   ├── link_repo.go         # Links CRUD
│   │   │   ├── postgres.go          # Connection pool
│   │   │   └── stats/               # Stats repository
│   │   │       └── stats_repo.go
│   │   └── redis/                   # Redis cache
│   ├── worker/                      # Worker pool (монолит, обратная совместимость)
│   └── model/                       # Доменные типы
├── docker-compose.yml               # PostgreSQL + Redis + Kafka + 3 сервиса
├── Dockerfile.link-service          # Dockerfile для Link Service
├── Dockerfile.stats-service         # Dockerfile для Stats Service
├── Dockerfile.gateway               # Dockerfile для API Gateway
├── Dockerfile                       # Dockerfile для монолита
├── go.mod
├── go.sum
└── README.md
```

Почему именно так: стандартная структура Go-проекта (см. [golang-standards/project-layout](https://github.com/golang-standards/project-layout)). Слои разделены, зависимости направлены внутрь (`handler → service → repository`). Легко тестировать и менять БД/фреймворки.

## План этапов

### Этап 0. Подготовка

- Настройка проекта (`go mod init`, структура папок).
- Docker-compose с PostgreSQL.
- Подключение к БД, проверка соединения.
- Миграции для таблицы `links`.

### Этап 1. MVP — базовый CRUD

- Модель `Link` (`id`, `short_code`, `original_url`, `created_at`, `clicks`, `last_accessed_at`).
- Генерация короткого кода: base62 от автоинкремента или случайная строка.
- HTTP-обработчики: `POST /api/v1/links`, `GET /{short}`, `GET /api/v1/links/{short}/stats`, `DELETE`.
- Обработка ошибок (404, 409, 400) с понятными JSON-ответами.
- Логирование через `slog`.
- Тесты: unit для сервиса, integration для handler через `httptest`.

### Этап 2. Улучшения

- Конфигурация через env + `godotenv`.
- Graceful shutdown HTTP-сервера (по сигналу SIGTERM).
- Индексы в БД на `short_code` (уникальный) и `original_url`.
- Метрики и health-check эндпоинты (`/health`, `/metrics`).
- Сборка Docker-образа, деплой через docker-compose.

### Этап 3. Кэш (Redis) ✅

- Почему кэш: 80% запросов — это `GET /{short}`, и они читают одни и те же данные.
- Cache-aside паттерн: сначала Redis, потом PostgreSQL.
- Инвалидация кэша при удалении.
- Обсуждение TTL и стратегий вытеснения.
- Graceful degradation при недоступности Redis.
- Метрики hit/miss/errors.

### Этап 4. Конкурентность ✅

- Асинхронная запись статистики через канал + воркер-пул.
- `sync.WaitGroup`, буферизированные каналы, graceful stop воркеров.
- Exponential backoff при ошибках БД.
- Обсуждение: когда каналы, а когда мьютексы.
- Метрики воркер-пула (queue size, processed, errors).

### Этап 5. Микросервисы ✅

- Разделение монолита на Link Service и Stats Service
- Apache Kafka для асинхронной коммуникации (события кликов)
- API Gateway для единой точки входа
- Graceful shutdown для producer/consumer
- Event-driven архитектура: ClickEvent → Kafka → Stats Service
- Обновлён `INTERVIEW.md`: Monolith vs Microservices, Kafka, CAP, Saga pattern

### Этап 6. Оптимизация (алгоритмы) ✅

- **HyperLogLog** (Redis PFADD/PFCOUNT) — уникальные переходы за O(1), 12KB на ключ
- **Redis Sorted Set** (ZINCRBY/ZREVRANGE) — топ ссылок за O(log n + K)
- **Sliding Window** (Redis pipeline + TTL) — trending ссылки в реальном времени
- **Бенчмарки генерации кодов:** CryptoRand 412 ns → Base62 from ID 7 ns (59x быстрее)
- Graceful degradation: Redis unavailable → fallback to PostgreSQL
- Обновлён `INTERVIEW.md`: HyperLogLog, Sorted Set, Big O, probabilistic structures

### Этап 7. Production-ready

- **Аутентификация и авторизация.** API-ключи или JWT. Разграничение доступа: кто может создавать, смотреть статистику, удалять ссылки. Обсуждение: когда API-ключ достаточно, а когда нужен OAuth2/OIDC.
- **Rate limiting.** Ограничение количества запросов на IP / на пользователя. Реализация через middleware: «ведро с дыркой» (token bucket) или «скользящее окно» — удобно реализовать на Redis. Обсуждение: как выбрать лимиты, что отдавать клиенту в заголовках (`X-RateLimit-*`).
- **CI/CD.** GitHub Actions: линтер (`golangci-lint`), unit- и integration-тесты на каждый PR, сборка Docker-образа. По желанию — деплой-пайплайн на любой хостинг.
- **Расширенная аналитика.** Сохранение `user-agent`, `referer`, IP (или гео по IP) при переходе. Построение простых отчётов: топ стран, топ устройств, топ рефереров.
