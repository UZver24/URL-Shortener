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

### ❌ Осталось сделать

- [ ] **Middleware: логирование запросов**
  - [ ] Обернуть `mux` в middleware
  - [ ] Логировать: method, path, status code, duration, request ID
  - [ ] Формат: structured logging через `slog`

- [ ] **Логирование через `slog`**
  - [ ] Настроить `slog.NewJSONHandler` или `slog.NewTextHandler` в `main.go`
  - [ ] Заменить `log.Println` на `slog.Info`/`slog.Error` во всех пакетах
  - [ ] Добавить request ID middleware (генерация UUID на каждый запрос)

- [ ] **Unit-тесты для сервиса**
  - [ ] `internal/service/link_service_test.go`
  - [ ] Мок-репозиторий (интерфейс `LinkRepository` уже есть в `link_service.go`)
  - [ ] Тесты: создание ссылки, дубликаты, коллизии, невалидный URL
  - [ ] Целевое покрытие: ≥70%

- [ ] **Integration-тесты для хендлеров**
  - [ ] `internal/handler/link_handler_test.go`
  - [ ] Использовать `httptest.NewServer`
  - [ ] Тестовая БД: поднять PostgreSQL через `testcontainers` или использовать `dktest`
  - [ ] Тесты: полный цикл create → redirect → stats → delete
  - [ ] Целевое покрытие: ≥80%

- [ ] **Собеседование по Этапу 1:**
  - [ ] Почему REST, а не gRPC?
  - [ ] HTTP 301 vs 302: в чём разница, что выбрать?
  - [ ] Как обрабатывать конкурентные запросы на один `short_code`?
  - [ ] Почему слоистая архитектура (handler → service → repository)?
  - [ ] Почему `errors.Is` вместо `==` при сравнении ошибок?
  - [ ] Что такое Dependency Injection и зачем он нужен?
  - [ ] Асинхронное обновление счётчика через goroutine — плюсы и минусы

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
- [ ] Все эндпоинты работают согласно спецификации (нужно протестировать вручную)
- [ ] Тесты проходят (`go test ./...`)
- [ ] Линтер не ругается (`golangci-lint run`)
- [ ] Middleware логирования запросов
- [ ] Приложение запускается через `docker-compose up` (с сервисом `api`)
- [x] Graceful shutdown работает (реализован)
- [ ] README обновлён с инструкцией "Как запустить"
- [ ] Код закоммичен в GitHub

---

## Следующий шаг

После завершения Фазы I переходим к **Фазе II — добавляем кэш (Redis)**.
