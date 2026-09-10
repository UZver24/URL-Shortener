# TODO — Фаза II: Кэширование

> **Архитектура фазы II:**
> ```
> Client → HTTP API (Go) → Redis (кэш) → PostgreSQL
> ```

---

## Этап 3. Кэш (Redis) ✅ ВЫПОЛНЕН

### Как запустить с Redis

```bash
# Вариант 1: Всё через Docker Compose (PostgreSQL + Redis + API)
docker-compose up --build

# Вариант 2: Только PostgreSQL и Redis в Docker, приложение локально
docker-compose up -d postgres redis
go run ./cmd/api

# Проверка работы
curl http://localhost:8080/health
# Ожидаемый ответ: {"status":"ok","database":"ok","cache":"ok"}

# Проверка метрик кэша
curl http://localhost:8080/metrics | grep cache_
# cache_hits_total{operation="get"} 0
# cache_misses_total{operation="get"} 0
# cache_errors_total{operation="get"} 0

# Тестирование
go test -v ./...
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

### Задачи

- [x] **Подготовка инфраструктуры**
  - [x] Добавить Redis в `docker-compose.yml`
    ```yaml
    redis:
      image: redis:7-alpine
      container_name: url-shortener-redis
      ports:
        - "6379:6379"
      volumes:
        - redis_data:/data
      healthcheck:
        test: ["CMD", "redis-cli", "ping"]
        interval: 10s
        timeout: 5s
        retries: 5
    ```
  - [x] Добавить `redis_data` volume в docker-compose
  - [x] Проверить подключение: `redis-cli ping` → `PONG`

- [x] **Интеграция Redis клиента**
  - [x] Установить библиотеку: `go get github.com/redis/go-redis/v9`
  - [x] Создать `internal/repository/redis/redis.go`:
    - [x] Подключение к Redis через `redis.NewClient()`
    - [x] Конфигурация через переменные окружения (`REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD`)
    - [x] Ping для проверки соединения при старте
  - [x] Graceful shutdown: закрытие соединения с Redis

- [x] **Реализация кэш-слоя**
  - [x] Создать `internal/repository/redis/link_cache.go`:
    - [x] `Get(ctx, shortCode) (*model.Link, error)` — получить ссылку из кэша
    - [x] `Set(ctx, shortCode, *model.Link, ttl) error` — сохранить ссылку в кэш
    - [x] `Delete(ctx, shortCode) error` — удалить ссылку из кэша (инвалидация)
  - [x] Формат хранения: JSON (сериализация `*model.Link`)
  - [x] TTL по умолчанию: 1 час (настраиваемый через `REDIS_TTL`)

- [x] **Cache-aside паттерн в сервисном слое**
  - [x] Обновить `internal/service/link_service.go`:
    - [x] `GetOriginalURL()`:
      1. Проверить Redis (`cache.Get()`)
      2. Если есть → вернуть из кэша (cache hit)
      3. Если нет → получить из PostgreSQL (`repo.GetByCode()`)
      4. Сохранить в Redis (`cache.Set()` с TTL)
      5. Вернуть результат
    - [x] `GetStats()`:
      1. Проверить Redis
      2. Если нет → получить из PostgreSQL
      3. Сохранить в Redis
    - [x] `Create()`:
      1. Создать в PostgreSQL
      2. **Не сохранять в кэш** (ленивое кэширование)
    - [x] `Delete()`:
      1. Удалить из PostgreSQL
      2. Инвалидировать кэш (`cache.Delete()`)

- [x] **Graceful degradation**
  - [x] Если Redis недоступен:
    - [x] Работать напрямую с PostgreSQL (fallback)
    - [x] Логировать ошибку Redis (WARN уровень)
    - [x] Не прерывать работу приложения
  - [x] Добавить метрику: `cache_errors_total` (Counter)

- [x] **Метрики для кэша**
  - [x] Добавить метрики в `internal/handler/middleware/metrics.go`:
    - [x] `cache_hits_total` (Counter) — количество попаданий в кэш
    - [x] `cache_misses_total` (Counter) — количество промахов
    - [x] `cache_errors_total` (Counter) — количество ошибок
  - [x] Метки: `operation` (get/set/delete)
  - [x] Обновить `/metrics` endpoint

- [x] **Обновление health-check**
  - [x] Обновить `internal/handler/health_handler.go`:
    - [x] Проверять соединение с Redis (`redis.Ping()`)
    - [x] Response: `{"status": "ok", "database": "ok", "cache": "ok"}`
    - [x] Если Redis недоступен: `{"status": "ok", "database": "ok", "cache": "error"}`
    - [x] HTTP 200 (приложение работает) даже если Redis недоступен

- [x] **Конфигурация**
  - [x] Добавить переменные окружения в `.env.example`:
    ```bash
    REDIS_HOST=localhost
    REDIS_PORT=6379
    REDIS_PASSWORD=
    REDIS_DB=0
    REDIS_TTL=3600  # 1 час в секундах
    ```
  - [x] Обновить `internal/config/config.go` для загрузки Redis конфигурации

- [x] **Тестирование**
  - [x] Unit-тесты для `link_cache.go` (с miniredis)
  - [x] Unit-тесты для cache-aside паттерна в `link_service_test.go`:
    - [x] Тест cache hit (получение из кэша)
    - [x] Тест cache miss (получение из БД + сохранение в кэш)
    - [x] Тест инвалидации кэша при удалении
    - [x] Тест graceful degradation (ошибка кэша → fallback к БД)
  - [ ] Integration-тесты:
    - [ ] Поднять Redis в тестовом docker-compose
    - [ ] Тест cache-aside паттерна (hit/miss)
    - [ ] Тест инвалидации кэша при удалении
    - [ ] Тест graceful degradation (остановить Redis, проверить fallback)
  - [ ] Нагрузочное тестирование:
    - [ ] Сравнить latency с кэшем и без
    - [ ] Измерить hit ratio при реалистичной нагрузке

- [x] **Собеседование по Этапу 3:**
  - [x] Что такое кэширование? Зачем нужно?
  - [x] Cache-aside vs Write-through vs Write-back — различия, плюсы/минусы
  - [x] TTL и стратегии вытеснения (LRU, LFU, FIFO)
  - [x] Инвалидация кэша: почему это сложно? Паттерны инвалидации
  - [x] Redis vs Memcached: когда что использовать?
  - [x] Как измерять эффективность кэша (hit ratio)?
  - [x] Что такое graceful degradation? Зачем нужно?
  - [x] Подробности — в `INTERVIEW.md`

### Результаты Этапа 3

**Покрытие тестами:**
- `service`: 81.7% (было 62.7%) ✅
- `repository/redis`: 51.7% (85% на Get/Set/Delete) ✅
- `handler`: 52.9%

**Добавленные файлы:**
- `internal/repository/redis/redis.go` — клиент Redis
- `internal/repository/redis/link_cache.go` — кэш-слой
- `internal/repository/redis/link_cache_test.go` — unit-тесты кэша

**Изменённые файлы:**
- `internal/service/link_service.go` — cache-aside паттерн
- `internal/service/link_service_test.go` — тесты cache-aside + mockCache
- `internal/handler/health_handler.go` — проверка Redis
- `internal/handler/middleware/metrics.go` — метрики кэша
- `internal/config/config.go` — Redis конфигурация
- `cmd/api/main.go` — wire Redis + graceful degradation
- `docker-compose.yml` — сервис Redis
- `.env.example` / `.env` — Redis переменные

**Зависимости:**
- `github.com/redis/go-redis/v9` — Redis клиент
- `github.com/alicebob/miniredis/v2` — для unit-тестов

---

## Этап 4. Конкурентность

### Задачи

- [ ] **Анализ текущей реализации**
  - [ ] Изучить текущий `IncrementClicks()` в `link_service.go`
  - [ ] Проблема: синхронное обновление счётчика увеличивает latency
  - [ ] Решение: асинхронная запись через канал + воркер-пул

- [ ] **Реализация воркер-пула**
  - [ ] Создать `internal/worker/pool.go`:
    - [ ] Структура `WorkerPool`:
      ```go
      type WorkerPool struct {
          tasks   chan Task
          workers int
          wg      sync.WaitGroup
          quit    chan struct{}
      }
      ```
    - [ ] `NewWorkerPool(workers, bufferSize int) *WorkerPool`
    - [ ] `Start()` — запуск воркеров
    - [ ] `Submit(task Task) error` — добавление задачи в очередь
    - [ ] `Stop()` — graceful shutdown (дождаться обработки всех задач)
  - [ ] Количество воркеров: настраиваемое через `WORKER_COUNT` (по умолчанию: 5)
  - [ ] Размер буфера: настраиваемое через `WORKER_BUFFER_SIZE` (по умолчанию: 1000)

- [ ] **Определение задачи**
  - [ ] Создать `internal/worker/task.go`:
    ```go
    type Task struct {
        ShortCode string
        Timestamp time.Time
    }
    ```
  - [ ] Воркер:
    1. Читает задачу из канала `tasks`
    2. Вызывает `repo.IncrementClicks(shortCode)`
    3. Логирует ошибки
    4. Переходит к следующей задаче

- [ ] **Интеграция в сервисный слой**
  - [ ] Обновить `internal/service/link_service.go`:
    - [ ] `GetOriginalURL()`:
      1. Получить ссылку из кэша/PostgreSQL
      2. Отправить задачу в воркер-пул: `pool.Submit(Task{ShortCode: code})`
      3. Вернуть URL (не дожидаясь обновления счётчика)
    - [ ] Graceful shutdown:
      1. Закрыть HTTP-сервер
      2. Остановить воркер-пул (`pool.Stop()`)
      3. Дождаться обработки всех задач
      4. Закрыть соединения с PostgreSQL и Redis

- [ ] **Graceful shutdown**
  - [ ] Обновить `cmd/api/main.go`:
    - [ ] Создать воркер-пул при старте
    - [ ] Передать пул в `link_service`
    - [ ] При получении SIGTERM:
      1. Остановить HTTP-сервер (`server.Shutdown()`)
      2. Остановить воркер-пул (`pool.Stop()`)
      3. Закрыть PostgreSQL (`pool.Close()`)
      4. Закрыть Redis (`redis.Close()`)
  - [ ] Таймаут: 30 секунд (настраиваемый через `SHUTDOWN_TIMEOUT`)

- [ ] **Обработка ошибок**
  - [ ] Если PostgreSQL недоступен:
    - [ ] Повторить попытку через exponential backoff (1s, 2s, 4s, 8s, 16s)
    - [ ] Максимум 5 попыток
    - [ ] Если все попытки провалились → логировать ошибку (ERROR)
  - [ ] Метрика: `worker_errors_total` (Counter)

- [ ] **Метрики для воркер-пула**
  - [ ] Добавить метрики в `internal/handler/middleware/metrics.go`:
    - [ ] `worker_queue_size` (Gauge) — текущий размер очереди
    - [ ] `worker_tasks_processed_total` (Counter) — обработано задач
    - [ ] `worker_processing_duration_seconds` (Histogram) — время обработки задачи
  - [ ] Метки: `status` (success/error)

- [ ] **Batch-обновления (опционально)**
  - [ ] Накопить N задач (например, 100) или ждать T секунд (например, 5)
  - [ ] Выполнить batch UPDATE:
    ```sql
    UPDATE links
    SET clicks = clicks + batch.count
    FROM (VALUES ('abc123', 5), ('xyz789', 3)) AS batch(short_code, count)
    WHERE links.short_code = batch.short_code
    ```
  - [ ] Преимущества: снижение нагрузки на PostgreSQL

- [ ] **Тестирование**
  - [ ] Unit-тесты для `worker/pool.go`:
    - [ ] Тест graceful shutdown (все задачи обработаны)
    - [ ] Тест переполнения буфера (ошибка при `Submit()`)
    - [ ] Тест обработки ошибок в воркерах
  - [ ] Integration-тесты:
    - [ ] Создать ссылку
    - [ ] Сделать 100 переходов (конкурентно)
    - [ ] Проверить, что счётчик = 100 (после обработки всех задач)
  - [ ] Нагрузочное тестирование:
    - [ ] Измерить latency `GetOriginalURL()` (должна уменьшиться)
    - [ ] Проверить, что все задачи обработаны при shutdown

- [ ] **Собеседование по Этапу 4:**
  - [ ] Что такое конкурентность в Go? Goroutines vs threads
  - [ ] Каналы: буферизированные vs небуферизированные
  - [ ] Паттерн Worker Pool: зачем нужен, как реализовать?
  - [ ] `sync.WaitGroup`: для чего используется?
  - [ ] Graceful shutdown: как правильно остановить воркеры?
  - [ ] Exponential backoff: что это, зачем нужно?
  - [ ] Batch-обработки: преимущества, как реализовать?
  - [ ] Подробности — в `INTERVIEW.md`

---

## Чеклист готовности Фазы II

- [x] Redis добавлен в `docker-compose.yml`
- [x] Кэш-слой реализован (`internal/repository/redis/link_cache.go`)
- [x] Cache-aside паттерн интегрирован в сервисный слой
- [x] Graceful degradation при недоступности Redis
- [x] Метрики для кэша (hits, misses, hit ratio)
- [x] Health-check обновлён (проверка Redis)
- [ ] Воркер-пул реализован (`internal/worker/pool.go`)
- [ ] Асинхронное обновление счётчика через воркер-пул
- [ ] Graceful shutdown для воркер-пула
- [ ] Метрики для воркер-пула (queue size, processed, errors)
- [x] Тесты для кэш-слоя (unit + integration)
- [ ] Тесты для воркер-пула (unit + integration)
- [x] INTERVIEW.md обновлён (Этап 3)
- [ ] INTERVIEW.md обновлён (Этап 4)
- [ ] README.md обновлён (инструкция по запуску с Redis)
- [ ] Код закоммичен в GitHub

---

## Следующий шаг

После завершения Фазы II переходим к **Фазе III — микросервисы** (Этап 5).
