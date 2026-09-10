# TODO — Фаза III: Микросервисы и Оптимизация

> **Текущая архитектура (Фаза II — завершена, тег `phase-ii-cache`):**
> ```
> Client → HTTP API (Go) → Redis (кэш) → PostgreSQL
>              ↓
>         Worker Pool (асинхронное обновление счётчика)
> ```

**Выполнено:**
- Этап 0. Подготовка ✅
- Этап 1. MVP — базовый CRUD ✅ (тег `v1.0.0-monolith`)
- Этап 2. Улучшения ✅
- Этап 3. Кэш (Redis) ✅
- Этап 4. Конкурентность ✅

---

## Этап 5. Микросервисы ✅

> **Цель:** Разделить монолит на Link Service (CRUD + redirect) и Stats Service (аналитика). Научиться работать с message broker для асинхронной коммуникации.

### Задачи

- [x] **Архитектурный анализ**
  - [x] Определить границы сервисов:
    - **Link Service:** `POST /api/v1/links`, `GET /{short}`, `DELETE /api/v1/links/{short}`
    - **Stats Service:** `GET /api/v1/links/{short}/stats`, `GET /api/v1/stats/top`, `GET /api/v1/stats/trending`
  - [x] Выбрать стратегию коммуникации:
    - **Асинхронная (Kafka):** для `IncrementClicks()` (не блокирует redirect)
  - [x] Определить формат событий (JSON — для простоты и наглядности)

- [x] **Подготовка инфраструктуры**
  - [x] Добавить Kafka (KRaft mode, без Zookeeper) в `docker-compose.yml`
  - [x] Настроить `docker-compose` для запуска трёх сервисов: `link-service`, `stats-service`, `gateway`
  - [x] Обновить `.env.example` для каждого сервиса

- [x] **Выделение Stats Service**
  - [x] Создать `cmd/stats-service/main.go` (отдельная точка входа)
  - [x] Перенести логику:
    - Обработка событий из Kafka (consumer)
    - `GetStats()` — получение статистики
    - `GetTopLinks()` — топ популярных ссылок
    - `GetTrending()` — "горячие" ссылки
  - [x] Создать отдельную схему в БД для статистики
  - [x] Миграции для таблиц `click_events` и `link_stats`

- [x] **Интеграция с Kafka**
  - [x] Установить библиотеку: `github.com/segmentio/kafka-go`
  - [x] Создать `internal/messaging/kafka/`:
    - `producer.go` — публикация событий (`PublishClick()`)
    - `consumer.go` — потребление событий с retry и exponential backoff
    - `events.go` — определение структуры события `ClickEvent`
  - [x] Graceful shutdown producer/consumer

- [x] **Обновление Link Service**
  - [x] При redirect → публиковать событие `ClickEvent` в Kafka
  - [x] Создать интерфейс `ClickPublisher` для абстракции от Kafka/воркер-пула
  - [x] Монолит (`cmd/api`) продолжает работать через `WorkerPoolClickPublisher` (обратная совместимость)

- [x] **API Gateway**
  - [x] Создать `cmd/gateway/` для единой точки входа
  - [x] Reverse proxy на Link/Stats Service
  - [x] Health-check gateway

- [x] **Тестирование**
  - [x] Unit-тесты для events (сериализация/десериализация)
  - [x] Обновлены тесты service и handler

- [x] **Документация и собеседование**
  - [x] Обновить `README.md`: архитектура микросервисов
  - [x] Обновить `AGENTS.md`: новая структура проекта
  - [x] Добавить блок в `INTERVIEW.md`:
    - Monolith vs Microservices: плюсы/минусы
    - Синхронная (REST/gRPC) vs асинхронная (Kafka) коммуникация
    - Event-driven архитектура
    - Kafka: producers, consumers, topics, partitions, offsets
    - Saga pattern для распределённых транзакций
    - CAP-теорема и её влияние на выбор БД
    - API Gateway
    - Graceful shutdown в микросервисах
    - JSON vs Protobuf
    - Мониторинг микросервисов

### Результаты (достигнутые)

**Новая архитектура:**
```
Client → Gateway (:8080) → Link Service (:8081) → PostgreSQL (links) → Redis (кэш)
                         ↘
                          Kafka (link.clicks)
                              ↓
                          Stats Service (:8082) → PostgreSQL (stats + click_events)
```

**Ключевые фичи:**
- ✅ Независимое масштабирование сервисов
- ✅ Изоляция сбоев (падение Stats не влияет на redirect)
- ✅ Асинхронная обработка статистики через Kafka
- ✅ Готовность к горизонтальному масштабированию
- ✅ Обратная совместимость: монолит (`cmd/api`) продолжает работать
- ✅ Graceful shutdown для всех компонентов
- ✅ Exponential backoff при ошибках обработки событий

---

## Этап 6. Оптимизация (алгоритмы)

> **Цель:** Применить алгоритмы и структуры данных для оптимизации. Оценить сложность решений в Big O. Использовать встроенные возможности Redis (HyperLogLog, Sorted Sets).

### Задачи

- [ ] **Генерация коротких кодов**
  - [ ] Текущая реализация: случайная строка base62 (6 символов, crypto/rand)
  - [ ] Сравнить с альтернативами:
    - **Base62 от ID (автоинкремент):** детерминированно, нет коллизий, но предсказуемо
    - **Snowflake ID (Twitter):** распределённая генерация, 64-bit, O(1)
    - **Counter + Base62 + obfuscation:** детерминированно + непредсказуемо
  - [ ] Реализовать и бенчмаркнуть каждый вариант
  - [ ] Оценка сложности: O(1) vs O(n) при коллизиях

- [ ] **Уникальные переходы (HyperLogLog)**
  - [ ] Проблема: `COUNT(DISTINCT user_id)` → O(n), много памяти
  - [ ] Решение: HyperLogLog на Redis:
    ```bash
    PFADD link:unique:{id} {user_id}
    PFCOUNT link:unique:{id}
    ```
    - Точность: ~0.81% ошибки при 1KB памяти
    - **Сложность:** O(1) добавление, O(1) получение
  - [ ] Интеграция в Stats Service:
    - При клике → `PFADD` в Redis
    - При статистике → `PFCOUNT` (можно объединять для диапазона дат)
  - [ ] Добавить поле `unique_clicks` в API статистики

- [ ] **Топ популярных ссылок (Sorted Set)**
  - [ ] Проблема: `ORDER BY clicks DESC LIMIT 10` → O(n log n)
  - [ ] Решение: Redis Sorted Set:
    ```bash
    ZINCRBY link:popularity 1 {link_id}
    ZREVRANGE link:popularity 0 9 WITHSCORES
    ```
    - **Сложность:** O(log n) обновление, O(k + log n) получение топ-k
  - [ ] Реализовать `GET /api/v1/stats/top?limit=10`
  - [ ] Альтернатива: Count-Min Sketch для частотного анализа

- [ ] **"Горячие" ссылки (Trending)**
  - [ ] Проблема: найти ссылки с всплеском активности за последний час
  - [ ] Решение: Sliding Window на Redis:
    ```bash
    # Окно 60 минут, гранулярность 1 минута
    INCR link:window:{id}:{minute}
    EXPIRE link:window:{id}:{minute} 3600
    # Сумма за окно:
    MGET link:window:{id}:{minute-1} ... link:window:{id}:{minute-60}
    ```
  - [ ] Реализовать `GET /api/v1/stats/trending`
  - [ ] Альтернатива: Redis TimeSeries (для более сложной аналитики)

- [ ] **Bloom Filter (опционально)**
  - [ ] Проблема: как быстро проверить, существует ли short_code?
  - [ ] Решение: Bloom Filter перед БД:
    - `false` → кода точно нет (не идти в БД)
    - `true` → возможно есть (идти в БД)
  - [ ] Реализация: Redis `BF.ADD`, `BF.EXISTS` (RedisBloom модуль)
  - [ ] Применение: защита от DoS-атак с несуществующими кодами

- [ ] **Нагрузочное тестирование**
  - [ ] Инструменты: `wrk`, `hey`, `vegeta`
  - [ ] Сценарии:
    - 10000 RPS на `GET /{short}` (redirect)
    - 1000 RPS на `POST /api/v1/links` (создание)
    - 100 RPS на `GET /api/v1/stats/top`
  - [ ] Измерить:
    - Latency (p50, p95, p99)
    - Throughput (RPS)
    - Memory usage (Redis, PostgreSQL)
  - [ ] Сравнить "до" и "после" оптимизаций

- [ ] **Документация и собеседование**
  - [ ] Добавить блок в `INTERVIEW.md`:
    - Base62 vs UUID vs Snowflake: когда что?
    - HyperLogLog: принцип работы, точность, применение
    - Probabilistic data structures: Count-Min Sketch, Bloom Filter
    - Big O notation: как оценить алгоритм
    - Premature optimization: когда оптимизировать, а когда нет

### Результаты (ожидаемые)

**Улучшения:**
- Уникальные переходы: O(n) → O(1), GB → KB памяти
- Топ ссылок: O(n log n) → O(k + log n)
- Генерация кодов: защита от коллизий + детерминизм
- Защита от DoS: Bloom Filter перед БД

---

## Этап 7. Production-ready

> **Цель:** Подготовить систему к production-использованию. Безопасность, наблюдаемость, CI/CD.

### Задачи

- [ ] **Аутентификация и авторизация**
  - [ ] Выбрать стратегию:
    - **API Keys:** просто, для server-to-server (рекомендуется для начала)
    - **JWT:** для user-facing приложений
    - **OAuth2/OIDC:** для интеграции с внешними провайдерами
  - [ ] Реализация API Keys:
    - Таблица `api_keys` (key_hash, user_id, scopes, created_at, expires_at)
    - Middleware для проверки заголовка `Authorization: Bearer <key>`
    - Кэширование валидных ключей в Redis
  - [ ] RBAC (Role-Based Access Control):
    - Скоупы: `links:create`, `links:delete`, `stats:read`
    - Проверка прав в middleware

- [ ] **Rate Limiting на Redis**
  - [ ] Использовать уже подключённый Redis (из Фазы II)
  - [ ] Алгоритм: **Token Bucket** (гибкий, допускает bursts)
    ```lua
    -- Lua script для атомарности (выполняется на стороне Redis)
    local tokens = redis.call('GET', key)
    -- ... логика пополнения и списания
    ```
  - [ ] Реализовать `internal/handler/middleware/ratelimit.go`:
    - Скользящее окно / token bucket
    - Лимиты по IP и API Key (раздельно)
    - Заголовки ответа: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`
  - [ ] HTTP 429 Too Many Requests при превышении
  - [ ] Конфигурация: `RATE_LIMIT_PER_IP`, `RATE_LIMIT_PER_KEY`

- [ ] **Наблюдаемость (Observability)**
  - [ ] **Логирование:**
    - Структурированные логи (JSON) — уже есть через `slog`
    - Request ID — уже есть
    - Добавить: correlation ID для микросервисов
  - [ ] **Метрики (уже частично есть):**
    - Prometheus — интегрирован
    - Добавить Grafana дашборды:
      - HTTP requests (RPS, latency p95/p99, error rate)
      - Cache hit ratio
      - Worker pool (queue size, processing time)
      - Kafka consumer lag (после Этапа 5)
    - Алерты: high error rate, high latency, queue overflow
  - [ ] **Distributed Tracing:**
    - OpenTelemetry SDK для Go
    - Jaeger для визуализации
    - Трассировка: HTTP → Service → Cache → DB → Kafka

- [ ] **CI/CD Pipeline (GitHub Actions)**
  - [ ] `.github/workflows/ci.yml`:
    ```yaml
    on: [push, pull_request]
    jobs:
      test:
        steps:
          - go test -race -v ./...
          - go vet ./...
          - gofmt -l .
          - golangci-lint run
          - go test -coverprofile=coverage.out ./...
      build:
        needs: test
        steps:
          - docker build -t url-shortener .
          - docker push
    ```
  - [ ] Quality gates:
    - `golangci-lint` (все линтеры)
    - `go test -race` (race detector)
    - Code coverage ≥ 80%
  - [ ] Deployment (опционально):
    - Kubernetes manifests
    - Helm charts
    - GitOps (ArgoCD/Flux)

- [ ] **Безопасность**
  - [ ] Валидация входных данных:
    - URL: whitelist протоколов (http, https)
    - Custom code: regex `^[a-zA-Z0-9_-]{3,20}$`
    - Защита от SSRF (Server-Side Request Forgery)
  - [ ] Защита от злоупотреблений:
    - Rate limiting (см. выше)
    - Blacklist доменов (фишинг, malware)
    - CAPTCHA для подозрительных запросов (опционально)
  - [ ] HTTPS/TLS:
    - Reverse proxy (nginx) с Let's Encrypt
    - HSTS заголовки
  - [ ] Секреты:
    - Не коммитить `.env` (уже в `.gitignore`)
    - HashiCorp Vault или AWS Secrets Manager (опционально)
    - Ротация API keys

- [ ] **Расширенная аналитика (опционально)**
  - [ ] Сохранение метаданных перехода:
    - `user_agent` → определение устройства/браузера
    - `referer` → источник перехода
    - IP → гео (через MaxMind GeoIP2)
    - `country`, `city`, `device_type`
  - [ ] Отчёты:
    - Топ стран
    - Топ устройств
    - Топ рефереров
  - [ ] Endpoint: `GET /api/v1/stats/{short}/detailed`

- [ ] **Документация и собеседование**
  - [ ] Обновить `README.md`: production deployment guide
  - [ ] Добавить блок в `INTERVIEW.md`:
    - Authentication: API Keys vs JWT vs OAuth2 — когда что?
    - Rate limiting алгоритмы: Token Bucket, Sliding Window, Fixed Window
    - Observability: три столпа (logs, metrics, traces)
    - CI/CD best practices
    - Security: OWASP Top 10 для веб-приложений

### Результаты (ожидаемые)

**Готовность к production:**
- ✅ Безопасность: аутентификация, rate limiting, HTTPS
- ✅ Наблюдаемость: логи, метрики, трассировка
- ✅ Автоматизация: CI/CD pipeline
- ✅ Документация: deployment guide, security best practices

---

## Чеклист готовности Фазы III

- [ ] Микросервисная архитектура (Link Service + Stats Service)
- [ ] Kafka для асинхронной коммуникации
- [ ] Оптимизация алгоритмов (Base62, HyperLogLog, Sorted Sets)
- [ ] Аутентификация и авторизация (API Keys)
- [ ] Rate limiting на Redis (Token Bucket)
- [ ] Наблюдаемость (Prometheus + Grafana + OpenTelemetry)
- [ ] CI/CD pipeline (GitHub Actions)
- [ ] Security best practices
- [ ] INTERVIEW.md обновлён (Этапы 5-7)
- [ ] README.md обновлён (production guide)
- [ ] Код закоммичен в GitHub

---

## Дальнейшее развитие (после Фазы III)

- **Масштабирование:** горизонтальное масштабирование, database sharding, read replicas
- **Геораспределение:** CDN, multi-region deployment, edge caching
- **Machine Learning:** предсказание популярных ссылок, детекция аномалий, спам-фильтр
- **Advanced аналитика:** ClickHouse для больших объёмов, real-time dashboards
