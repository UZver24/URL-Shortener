# TODO — Фаза I: Монолит

> **Архитектура фазы I:**
> ```
> Client → HTTP API (Go) → PostgreSQL
> ```

---

## Этап 0. Подготовка ✅

- [x] **Инициализация Go-модуля**
  - [x] `go mod init github.com/UZver24/URL-Shortener`
  - [x] Создана структура папок:
    - `cmd/api/` — точка входа
    - `internal/config/` — конфигурация
    - `internal/handler/` — HTTP-обработчики
    - `internal/service/` — бизнес-логика
    - `internal/repository/postgres/` — работа с БД
    - `internal/model/` — доменные типы
    - `cmd/api/migrations/` — SQL-миграции (перенесены сюда для `//go:embed`)

- [x] **Docker-инфраструктура**
  - [x] `docker-compose.yml` с PostgreSQL 16-alpine
  - [x] Volume `postgres_data` для персистентности
  - [x] Healthcheck в docker-compose (`pg_isready`)

- [x] **Подключение к БД**
  - [x] Зависимость `github.com/jackc/pgx/v5`
  - [x] `internal/config/config.go` — загрузка переменных окружения
  - [x] `internal/repository/postgres/postgres.go` — пул соединений (`pgxpool`)
  - [x] Smoke-тест: `pool.Ping(ctx)` при создании пула

- [x] **Миграции**
  - [x] Установлен `golang-migrate/migrate/v4`
  - [x] `cmd/api/migrations/000001_init_links.up.sql` — создание таблицы + индексы
  - [x] `cmd/api/migrations/000001_init_links.down.sql` — откат
  - [x] Автоматический запуск миграций через `//go:embed` в `main.go`

- [x] **Дополнительно**
  - [x] `.gitignore` — бинарники, `.env`, IDE-файлы
  - [x] `.env.example` + `.env` — шаблон и локальная конфигурация

---

## Этап 1. MVP — базовый CRUD

### ✅ Выполнено

- [x] **Модель `Link`** — `internal/model/link.go`
  - [x] `Link` — доменная модель (`id`, `short_code`, `original_url`, `created_at`, `clicks`, `last_accessed_at`)
  - [x] `CreateLinkRequest` — DTO для POST-запроса (с опциональным `custom_code`)
  - [x] `CreateLinkResponse` — DTO для ответа создания
  - [x] `LinkStatsResponse` — DTO для статистики

- [x] **Кастомные ошибки** — `internal/model/errors.go`
  - `ErrLinkNotFound`, `ErrLinkAlreadyExists`, `ErrShortCodeAlreadyTaken`, `ErrInvalidInput`, `ErrInternalServer`

- [x] **Генерация короткого кода** — в `internal/service/link_service.go`
  - [x] `generateShortCode()` — случайная строка 6 символов, `crypto/rand`, base62-алфавит
  - [x] Поддержка пользовательского кода через `custom_code`
  - [x] Retry-логика при коллизии (до 3 попыток)
  - [ ] ⚠️ Обсудить: base62 от автоинкремента vs случайная строка (пока только случайная)

- [x] **Репозиторий** — `internal/repository/postgres/link_repo.go`
  - [x] `Create(ctx, link)` — с `RETURNING id` и обработкой дубликатов (SQLSTATE 23505)
  - [x] `GetByCode(ctx, code)` — поиск по short_code
  - [x] `GetByOriginalURL(ctx, url)` — поиск дубликата оригинального URL
  - [x] `IncrementClicks(ctx, id)` — обновление счётчика и `last_accessed_at`
  - [x] `Delete(ctx, code)` — удаление с проверкой `RowsAffected`
  - [x] Обработка `pgx.ErrNoRows` → `ErrLinkNotFound`

- [x] **Сервисный слой** — `internal/service/link_service.go`
  - [x] `CreateLink(ctx, url, customCode)` — валидация, проверка дубликатов, генерация кода
  - [x] `GetOriginalURL(ctx, code)` — возврат URL + **асинхронное** увеличение счётчика через goroutine
  - [x] `GetStats(ctx, code)` — статистика
  - [x] `DeleteLink(ctx, code)` — удаление
  - [x] `isValidURL()` — валидация URL (схема + хост)

- [x] **HTTP-обработчики** — `internal/handler/link_handler.go`
  - [x] `POST /api/v1/links` → 201 Created / 400 / 409
  - [x] `GET /{short}` → 302 Found / 404
  - [x] `GET /api/v1/links/{short}/stats` → 200 / 404
  - [x] `DELETE /api/v1/links/{short}` → 204 / 404
  - [x] Маршрутизация через `net/http.ServeMux` (Go 1.22+)
  - [x] `respondJSON`, `respondError` — JSON-ответы
  - [x] `handleServiceError` — маппинг ошибок сервиса на HTTP-коды через `errors.Is`

- [x] **Точка входа** — `cmd/api/main.go`
  - [x] Dependency injection: `repo → service → handler`
  - [x] Запуск миграций, HTTP-сервера
  - [x] Graceful shutdown (SIGINT/SIGTERM)

- [x] **Middleware: логирование запросов**
  - [x] `internal/handler/middleware/logging.go` — middleware для логирования
  - [x] `internal/handler/middleware/requestid.go` — генерация UUID для каждого запроса
  - [x] Обёрнут `mux` в цепочку middleware: `RequestID → Logging → mux`
  - [x] Логируются: method, path, status code, duration_ms, user_agent, remote_addr, request_id
  - [x] Формат: structured logging через `slog` (JSON)

- [x] **Логирование через `slog`**
  - [x] Настроен `slog.NewJSONHandler` в `main.go`
  - [x] Заменены `log.Println` на `slog.Info`/`slog.Error`/`slog.Warn` во всех пакетах
  - [x] Request ID middleware генерирует UUID для каждого запроса
  - [x] Request ID добавляется в контекст и заголовок `X-Request-ID`

- [x] **Unit-тесты для сервиса**
  - [x] `internal/service/link_service_test.go` — 12 тестов
  - [x] Мок-репозиторий реализован
  - [x] Тесты: создание ссылки, дубликаты, коллизии, невалидный URL, custom_code, удаление
  - [x] Покрытие: **84.3%** (целевое ≥70%)

- [x] **Integration-тесты для хендлеров**
  - [x] `internal/handler/link_handler_test.go` — 11 тестов
  - [x] Использован `httptest.NewRecorder`
  - [x] Тесты: все эндпоинты + обработка ошибок (400, 404, 409)
  - [x] Покрытие: **88.5%** (целевое ≥80%)

- [x] **Собеседование по Этапу 1:**
  - [x] Почему REST, а не gRPC?
  - [x] HTTP 301 vs 302: в чём разница, что выбрать?
  - [x] Как обрабатывать конкурентные запросы на один `short_code`?
  - [x] Почему слоистая архитектура (handler → service → repository)?
  - [x] Почему `errors.Is` вместо `==` при сравнении ошибок?
  - [x] Что такое Dependency Injection и зачем он нужен?
  - [x] Асинхронное обновление счётчика через goroutine — плюсы и минусы
  - [x] Подробности и ответы — в файле `INTERVIEW.md`

---

## Этап 2. Улучшения

### ✅ Выполнено (раньше плана)

- [x] **Конфигурация через переменные окружения** (изначально планировалось здесь)
  - [x] `github.com/joho/godotenv`
  - [x] `.env.example` + `.env`
  - [x] Загрузка с валидацией в `config/config.go`

- [x] **Graceful shutdown** (изначально планировалось здесь)
  - [x] Обработка `SIGINT`, `SIGTERM` через `signal.Notify`
  - [x] `http.Server.Shutdown(ctx)` с таймаутом 30 секунд
  - [x] `defer pool.Close()` — закрытие пула соединений

- [x] **Индексы в БД** (добавлены сразу в первую миграцию)
  - [x] `UNIQUE INDEX idx_links_short_code`
  - [x] `INDEX idx_links_original_url`
  - [ ] ⚠️ Обсудить: B-tree vs Hash, когда какой использовать

### ❌ Осталось сделать

- [ ] **Health-check эндпоинт**
  - [ ] `GET /health` — проверка готовности
  - [ ] Response: `{"status": "ok"}` или `{"status": "error", "reason": "..."}`
  - [ ] Проверка соединения с БД через `pool.Ping()`

- [ ] **Prometheus-метрики (опционально)**
  - [ ] `GET /metrics` — эндпоинт для Prometheus
  - [ ] Счётчик запросов по методам и статусам
  - [ ] Гистограмма latency
  - [ ] Количество активных соединений с БД
  - [ ] `github.com/prometheus/client_golang/prometheus`

- [ ] **Dockerfile (multi-stage build)**
  - [ ] Stage 1: `golang:1.22-alpine` — сборка бинарника
  - [ ] Stage 2: `alpine:latest` — минимальный образ для запуска
  - [ ] `CGO_ENABLED=0` для статического бинарника
  - [ ] Обновить `docker-compose.yml`: добавить сервис `api`
  - [ ] Проверить: `docker-compose build`, `docker-compose up`

- [ ] **Собеседование по Этапу 2:**
  - [ ] Что такое multi-stage build в Docker? Зачем он?
  - [ ] Разница между liveness probe и readiness probe
  - [ ] Как работает graceful shutdown в Kubernetes?
  - [ ] Что такое индексы в PostgreSQL? B-tree vs Hash
  - [ ] Когда индекс вреден?

---

## Этапы 3–7 (без изменений)

> См. README.md — этапы 3 (кэш), 4 (конкурентность), 5 (микросервисы), 6 (оптимизация), 7 (production-ready).

---

## Чеклист готовности Фазы I

- [x] Базовая структура проекта создана
- [x] Приложение компилируется (`go build ./cmd/api` → `url-shortener`, 17MB)
- [x] Все эндпоинты работают согласно спецификации (протестировано вручную)
  - `POST /api/v1/links` → 201 Created
  - `GET /{short}` → 302 Found
  - `GET /api/v1/links/{short}/stats` → 200 OK
  - `DELETE /api/v1/links/{short}` → 204 No Content
  - Обработка ошибок: 400, 404, 409
- [x] Тесты проходят (`go test ./...`)
  - Service: 12 тестов, покрытие 84.3%
  - Handler: 11 тестов, покрытие 88.5%
- [x] Линтер не ругается
  - `gofmt` — ✅
  - `go vet` — ✅
  - `staticcheck` — ✅
  - `golangci-lint` — ⚠️ typecheck ошибки (известная проблема v1.64+, не связана с кодом)
- [x] Middleware логирования запросов (RequestID + Logging через slog)
- [ ] Приложение запускается через `docker-compose up` (с сервисом `api`) — требуется Dockerfile (Этап 2)
- [x] Graceful shutdown работает (реализован)
- [x] README обновлён с инструкцией "Как запустить" (Quick Start)
- [x] INTERVIEW.md создан с вопросами/ответами для собеседования
- [ ] Код закоммичен в GitHub (пользователь коммитит самостоятельно)

---

## Следующий шаг

После завершения Фазы I переходим к **Фазе II — добавляем кэш (Redis)**.

---

## Итоги Этапа 1

### Что сделано

| Компонент | Статус | Покрытие |
|-----------|--------|----------|
| **Модель `Link`** | ✅ | — |
| **Кастомные ошибки** | ✅ | — |
| **Генерация short_code** | ✅ | — |
| **Репозиторий (CRUD)** | ✅ | — |
| **Сервисный слой** | ✅ | 84.3% |
| **HTTP-обработчики** | ✅ | 88.5% |
| **Middleware (RequestID + Logging)** | ✅ | — |
| **Точка входа (main.go)** | ✅ | — |
| **Unit-тесты (12 тестов)** | ✅ | 84.3% |
| **Integration-тесты (11 тестов)** | ✅ | 88.5% |
| **INTERVIEW.md** | ✅ | 11 вопросов |
| **README (Quick Start)** | ✅ | — |

### Файлы проекта

```
URL-Shortener/
├── cmd/api/
│   ├── main.go                              # Точка входа
│   └── migrations/
│       ├── 000001_init_links.up.sql
│       └── 000001_init_links.down.sql
├── internal/
│   ├── config/config.go                     # Конфигурация
│   ├── handler/
│   │   ├── link_handler.go                  # HTTP-обработчики
│   │   ├── link_handler_test.go             # 11 тестов
│   │   └── middleware/
│   │       ├── requestid.go                 # Request ID middleware
│   │       └── logging.go                   # Logging middleware
│   ├── model/
│   │   ├── link.go                          # Доменная модель + DTO
│   │   └── errors.go                        # Кастомные ошибки
│   ├── repository/postgres/
│   │   ├── postgres.go                      # Пул соединений
│   │   └── link_repo.go                     # CRUD-операции
│   └── service/
│       ├── link_service.go                  # Бизнес-логика
│       └── link_service_test.go             # 12 тестов
├── docker-compose.yml                       # PostgreSQL
├── .env.example                             # Шаблон конфигурации
├── .env                                     # Локальная конфигурация
├── .gitignore                               # Игнорируемые файлы
├── .golangci.yml                            # Конфигурация линтера
├── go.mod                                   # Зависимости
├── go.sum                                   # Контрольные суммы
├── README.md                                # Документация + Quick Start
├── TODO.md                                  # План задач
└── INTERVIEW.md                             # Вопросы для собеседования
```

### Команды для проверки

```bash
# Запустить тесты
go test -v ./...

# Проверить покрытие
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# Запустить линтеры
gofmt -l .
go vet ./...
staticcheck ./...

# Собрать бинарник
go build -o url-shortener ./cmd/api

# Запустить приложение
docker-compose up -d
go run ./cmd/api
```

### Готово к коммиту

Этап 1 завершён. Можно коммитить в GitHub:

```bash
git add .
git commit -m "feat: complete Etap 1 — MVP with tests and logging"
git push
```
