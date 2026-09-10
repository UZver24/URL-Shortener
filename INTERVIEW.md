# Собеседование по Этапу 1: MVP — базовый CRUD

> Документация с типичными вопросами на техническом собеседовании и подробными ответами по теме первого этапа.

---

## Архитектура и дизайн

### 1. Почему REST, а не gRPC?

**Ответ:**

Для URL-сокращателя на этапе MVP REST — оптимальный выбор по нескольким причинам:

| Критерий | REST | gRPC |
|----------|------|------|
| **Простота** | Простой HTTP + JSON, легко отлаживать через curl/Postman | Требует protobuf-схемы, бинарный формат |
| **Клиенты** | Любой HTTP-клиент (браузер, curl, мобильное приложение) | Нужны сгенерированные клиенты для каждого языка |
| **Редирект** | Нативный HTTP 302 + `Location` header | Не подходит для редиректов в браузере |
| **Экосистема** | Встроенная поддержка в браузерах, CDN, балансировщиках | Требует gRPC-proxy для веб-трафика |

**Когда gRPC лучше:**
- Межсервисное взаимодействие (внутренние микросервисы)
- Высокая нагрузка (protobuf компактнее JSON в 3-10 раз)
- Streaming (двунаправленный поток данных)
- Строгая типизация через protobuf-схемы

**Для нашего проекта:**
- `GET /{short}` должен работать как обычный редирект в браузере — gRPC не подходит
- API публичный, клиенты будут разные (curl, браузеры, мобильные приложения)
- Нагрузка на этапе MVP низкая, оптимизация не нужна

**Follow-up вопрос:** А если бы у нас было 2 сервиса (Link Service и Stats Service), как бы они общались?

**Ответ:** Между внутренними сервисами уже можно использовать gRPC — это закрытое API, клиенты контролируемые, нагрузка может быть высокой. Это классический паттерн: REST для внешнего API (BFF — Backend for Frontend), gRPC для внутреннего (service-to-service).

---

### 2. HTTP 301 vs 302: в чём разница, что выбрать?

**Ответ:**

| Код | Название | Поведение | Кэширование |
|-----|----------|-----------|-------------|
| **301** | Moved Permanently | Постоянный редирект | Браузер кэширует навсегда |
| **302** | Found | Временный редирект | Браузер всегда обращается к серверу |
| **307** | Temporary Redirect | Как 302, но сохраняет метод (POST → POST) | Не кэшируется |
| **308** | Permanent Redirect | Как 301, но сохраняет метод | Кэшируется |

**Почему выбрали 302:**

1. **Гибкость:** Если ссылка удалена, браузер не будет пытаться перейти по старому URL (как было бы с 301)
2. **Статистика:** Каждый переход логируется на сервере (при 301 браузер может кэшировать редирект и не обращаться к нам)
3. **Безопасность:** Если короткая ссылка была скомпрометирована, мы можем её удалить, и браузер не будет кэшировать старый редирект

**Когда использовать 301:**
- Постоянные редиректы (например, переезд домена `old.com → new.com`)
- SEO-оптимизация (поисковики переносят PageRank при 301)
- Когда уверены, что URL никогда не изменится

**Для URL-сокращателя 302 — правильный выбор.**

---

### 3. Почему слоистая архитектура (handler → service → repository)?

**Ответ:**

Слоистая архитектура (Clean Architecture / Hexagonal Architecture) разделяет ответственность:

```
Handler (HTTP) → Service (бизнес-логика) → Repository (БД)
     ↓                    ↓                      ↓
  HTTP-контекст     Доменные правила         SQL-запросы
```

**Преимущества:**

| Слой | Ответственность | Пример |
|------|-----------------|--------|
| **Handler** | HTTP: парсинг запроса, валидация JSON, HTTP-коды | Декодировать JSON → вызвать сервис → вернуть 201/400 |
| **Service** | Бизнес-логика: правила, валидация, генерация кода | Проверить дубликат URL, сгенерировать short_code |
| **Repository** | Работа с БД: SQL-запросы, транзакции | `INSERT INTO links ... RETURNING id` |

**Почему это важно:**

1. **Тестируемость:** Можно тестировать сервис без БД (мок-репозиторий), хендлеры без реального HTTP-сервера (`httptest`)
2. **Заменяемость:** Можно поменять PostgreSQL на MongoDB, изменив только `repository`, не трогая `service` и `handler`
3. **Переиспользование:** Сервисную логику можно использовать из других мест (CLI, gRPC, cron-задачи)
4. **Читаемость:** Каждый слой имеет чёткую ответственность, код легче понять

**Пример из нашего кода:**

```go
// Handler: HTTP-контекст
func (h *LinkHandler) CreateLink(w http.ResponseWriter, r *http.Request) {
    var req model.CreateLinkRequest
    json.NewDecoder(r.Body).Decode(&req)  // HTTP: парсинг JSON
    
    link, err := h.linkService.CreateLink(r.Context(), req.URL, req.CustomCode)  // вызов сервиса
    if err != nil {
        handleServiceError(w, err)  // HTTP: маппинг ошибок на коды
        return
    }
    
    respondJSON(w, http.StatusCreated, resp)  // HTTP: формирование ответа
}

// Service: бизнес-логика
func (s *LinkService) CreateLink(ctx context.Context, url, customCode string) (*model.Link, error) {
    if !isValidURL(url) {  // бизнес-правило: валидация
        return nil, model.ErrInvalidInput
    }
    
    existing, _ := s.repo.GetByOriginalURL(ctx, url)  // бизнес-правило: проверка дубликатов
    if existing != nil {
        return existing, nil
    }
    
    shortCode := generateShortCode()  // бизнес-логика: генерация кода
    // ...
}

// Repository: работа с БД
func (r *LinkRepository) Create(ctx context.Context, link *model.Link) error {
    query := `INSERT INTO links ... RETURNING id`  // SQL
    return r.pool.QueryRow(ctx, query, ...).Scan(&link.ID)
}
```

**Альтернативы:**
- **Flat structure** (всё в handler) — быстро, но не масштабируется
- **Domain-Driven Design** (агрегаты, value objects) — избыточно для MVP

---

### 4. Почему `errors.Is` вместо `==` при сравнении ошибок?

**Ответ:**

В Go 1.13+ появился механизм **error wrapping** — обёртывание ошибок с сохранением цепочки:

```go
// Без wrapping
return ErrLinkNotFound

// С wrapping (добавляем контекст)
return fmt.Errorf("failed to get link: %w", ErrLinkNotFound)
```

**Проблема с `==`:**

```go
err := fmt.Errorf("failed: %w", ErrLinkNotFound)

if err == ErrLinkNotFound {  // ❌ false!
    // Не сработает, потому что err — это обёрнутая ошибка
}
```

**Решение с `errors.Is`:**

```go
if errors.Is(err, ErrLinkNotFound) {  // ✅ true!
    // Работает, потому что errors.Is проверяет цепочку ошибок
}
```

**Как работает `errors.Is`:**

```go
func errors.Is(err, target error) bool {
    for {
        if err == target {
            return true
        }
        // Проверяем, реализует ли err интерфейс Unwrap()
        unwrapper, ok := err.(interface{ Unwrap() error })
        if !ok {
            return false
        }
        err = unwrapper.Unwrap()  // Распаковываем и продолжаем
    }
}
```

**Пример из нашего кода:**

```go
// В repository
func (r *LinkRepository) GetByCode(ctx context.Context, code string) (*model.Link, error) {
    // ...
    if errors.Is(err, pgx.ErrNoRows) {  // Проверяем ошибку БД
        return nil, model.ErrLinkNotFound
    }
    return nil, fmt.Errorf("select link: %w", err)  // Wrap с контекстом
}

// В handler
func handleServiceError(w http.ResponseWriter, err error) {
    switch {
    case errors.Is(err, model.ErrLinkNotFound):  // Работает даже если ошибка обёрнута
        respondError(w, http.StatusNotFound, "Link not found")
    // ...
    }
}
```

**Когда использовать:**
- `==` — только для простых ошибок-констант без wrapping
- `errors.Is` — всегда, когда ошибка может быть обёрнута (best practice)
- `errors.As` — когда нужно извлечь конкретный тип ошибки (например, `*NotFoundError`)

---

### 5. Что такое Dependency Injection и зачем он нужен?

**Ответ:**

**Dependency Injection (DI)** — паттерн, при котором зависимости передаются компоненту извне, а не создаются внутри него.

**Без DI (плохо):**

```go
type LinkService struct {
    repo *postgres.LinkRepository  // Жёсткая зависимость
}

func NewLinkService() *LinkService {
    return &LinkService{
        repo: postgres.NewLinkRepository(),  // Создаём внутри
    }
}
```

**Проблемы:**
- Невозможно подменить `repo` на мок для тестов
- Сервис зависит от конкретной реализации PostgreSQL
- Сложно менять конфигурацию (например, использовать SQLite в тестах)

**С DI (хорошо):**

```go
// Определяем интерфейс
type LinkRepository interface {
    Create(ctx context.Context, link *model.Link) error
    GetByCode(ctx context.Context, code string) (*model.Link, error)
    // ...
}

type LinkService struct {
    repo LinkRepository  // Зависимость от интерфейса
}

// Зависимость передаётся извне
func NewLinkService(repo LinkRepository) *LinkService {
    return &LinkService{repo: repo}
}
```

**Преимущества:**

1. **Тестируемость:**
   ```go
   // В тестах передаём мок
   mockRepo := newMockRepository()
   svc := NewLinkService(mockRepo)
   ```

2. **Гибкость:**
   ```go
   // В main.go используем PostgreSQL
   pgRepo := postgres.NewLinkRepository(pool)
   svc := NewLinkService(pgRepo)
   
   // В CLI-утилите можно использовать SQLite
   sqliteRepo := sqlite.NewLinkRepository(db)
   svc := NewLinkService(sqliteRepo)
   ```

3. **Inversion of Control (IoC):**
   - Сервис не знает, какую БД использует
   - `main.go` решает, какие компоненты связать

**Пример из нашего кода (`cmd/api/main.go`):**

```go
func main() {
    // 1. Создаём зависимости
    pool, _ := postgres.NewPool(cfg.DatabaseURL())
    linkRepo := postgres.NewLinkRepository(pool)
    
    // 2. Inject в сервис
    linkService := service.NewLinkService(linkRepo, logger)
    
    // 3. Inject в handler
    linkHandler := handler.NewLinkHandler(linkService)
    
    // ...
}
```

**DI-фреймворки:**
- **Uber fx** — DI-контейнер для Go
- **Google Wire** — code generation для DI

Для нашего проекта ручной DI достаточно — всего 3 компонента.

---

### 6. Как обрабатывать конкурентные запросы на один `short_code`?

**Ответ:**

**Сценарий:** 1000 запросов одновременно обращаются к `GET /abc123`.

**Что происходит:**

1. **Handler** получает 1000 параллельных запросов
2. **Service** вызывает `repo.GetByCode()` для каждого
3. **Repository** выполняет 1000 `SELECT` запросов к БД
4. **Service** асинхронно увеличивает счётчик через goroutine

**Проблемы:**

| Проблема | Причина | Решение |
|----------|---------|---------|
| **Race condition** | Несколько goroutine одновременно обновляют `clicks` | Атомарный `UPDATE clicks = clicks + 1` |
| **Потеря данных** | Счётчик может быть неточным | Приемлемо для статистики |
| **Нагрузка на БД** | 1000 `UPDATE` за секунду | Буферизация, batch update |

**Как мы решили:**

```go
// В service
func (s *LinkService) GetOriginalURL(ctx context.Context, code string) (string, error) {
    link, err := s.repo.GetByCode(ctx, code)  // Синхронно
    if err != nil {
        return "", err
    }

    // Асинхронно увеличиваем счётчик
    go func() {
        bgCtx := context.Background()
        _ = s.repo.IncrementClicks(bgCtx, link.ID)  // Fire and forget
    }()

    return link.OriginalURL, nil
}
```

**Почему асинхронно:**
- Не блокируем HTTP-ответ ожиданием `UPDATE`
- Клиент получает редирект быстрее
- Если `UPDATE` упал — не критично (статистика, не бизнес-логика)

**Почему `UPDATE clicks = clicks + 1` атомарен:**
- PostgreSQL использует MVCC (Multi-Version Concurrency Control)
- `UPDATE` блокирует строку до завершения транзакции
- Два одновременных `UPDATE` выполняются последовательно

**Улучшения для продакшена:**

1. **Буферизация в памяти:**
   ```go
   clicksBuffer := make(chan int64, 10000)
   
   // В handler
   clicksBuffer <- link.ID
   
   // Воркер
   go func() {
       for id := range clicksBuffer {
           repo.IncrementClicks(bgCtx, id)
       }
   }()
   ```

2. **Batch update:**
   ```sql
   UPDATE links 
   SET clicks = clicks + count 
   FROM (VALUES (1, 5), (2, 3)) AS updates(id, count) 
   WHERE links.id = updates.id
   ```

3. **Redis для счётчиков:**
   ```go
   redis.Incr(ctx, "link:abc123:clicks")
   ```

---

### 7. Асинхронное обновление счётчика через goroutine — плюсы и минусы

**Ответ:**

**Плюсы:**

| Преимущество | Описание |
|--------------|----------|
| **Низкая latency** | Клиент не ждёт `UPDATE` запроса |
| **Простота** | Одна строка `go func() { ... }()` |
| **Изоляция ошибок** | Если `UPDATE` упал, редирект всё равно работает |

**Минусы:**

| Недостаток | Описание |
|------------|----------|
| **Потеря данных** | При crash приложения goroutine умирает, счётчик не обновится |
| **Нет контроля** | Нельзя отследить, сколько goroutine запущено |
| **Утечка goroutine** | Если БД недоступна, goroutine накапливаются |
| **Нет graceful shutdown** | При остановке сервера активные goroutine прерываются |

**Пример утечки:**

```go
// Если БД недоступна, каждая goroutine зависает на таймауте
go func() {
    _ = s.repo.IncrementClicks(bgCtx, link.ID)  // Блокируется на 30 секунд
}()

// За 1 минуту: 1000 запросов × 30 сек = 1000 зависших goroutine
```

**Улучшенная версия:**

```go
type LinkService struct {
    repo LinkRepository
    clicksChan chan int64  // Буферизованный канал
}

func NewLinkService(repo LinkRepository) *LinkService {
    s := &LinkService{
        repo: repo,
        clicksChan: make(chan int64, 10000),  // Буфер на 10000 элементов
    }
    
    // Запускаем воркер
    go s.clicksWorker()
    
    return s
}

func (s *LinkService) clicksWorker() {
    for id := range s.clicksChan {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        _ = s.repo.IncrementClicks(ctx, id)
        cancel()
    }
}

func (s *LinkService) GetOriginalURL(ctx context.Context, code string) (string, error) {
    link, err := s.repo.GetByCode(ctx, code)
    if err != nil {
        return "", err
    }

    // Отправляем в канал (не блокирует, если буфер не полон)
    select {
    case s.clicksChan <- link.ID:
    default:
        // Буфер полон — логируем и пропускаем
        slog.Warn("clicks buffer full, dropping click", "link_id", link.ID)
    }

    return link.OriginalURL, nil
}

func (s *LinkService) Shutdown() {
    close(s.clicksChan)  // Останавливаем воркер
}
```

**Преимущества:**
- Контроль над количеством goroutine (один воркер)
- Буферизация защищает от всплесков нагрузки
- Graceful shutdown через `close(clicksChan)`

**Для продакшена:**
- Kafka/RabbitMQ для надёжной очереди
- Redis для счётчиков (быстрее, чем PostgreSQL)
- ClickHouse для аналитики (оптимизирован для счётчиков)

---

## Базы данных

### 8. Что такое индексы в PostgreSQL? B-tree vs Hash

**Ответ:**

**Индекс** — структура данных, ускоряющая поиск строк по определённому столбцу (как алфавитный указатель в книге).

**Без индекса:**
```sql
SELECT * FROM links WHERE short_code = 'abc123';
-- Sequential Scan: читает ВСЮ таблицу (миллионы строк)
```

**С индексом:**
```sql
CREATE INDEX idx_links_short_code ON links(short_code);
-- Index Scan: читает только нужную строку (O(log N))
```

**Типы индексов:**

| Тип | Структура | Когда использовать | Сложность |
|-----|-----------|-------------------|-----------|
| **B-tree** | Сбалансированное дерево | По умолчанию, диапазоны, сортировка | O(log N) |
| **Hash** | Хеш-таблица | Только точное совпадение (`=`) | O(1) |
| **GIN** | Инвертированный индекс | Полнотекстовый поиск, JSONB | — |
| **BRIN** | Блочный индекс | Большие таблицы с упорядоченными данными | — |

**B-tree vs Hash:**

| Критерий | B-tree | Hash |
|----------|--------|------|
| **Точное совпадение** (`=`) | ✅ | ✅ (быстрее) |
| **Диапазоны** (`>`, `<`, `BETWEEN`) | ✅ | ❌ |
| **Сортировка** (`ORDER BY`) | ✅ | ❌ |
| **Размер на диске** | Больше | Меньше |
| **WAL (журнал)** | Полная поддержка | Ограниченная (PostgreSQL 10+) |

**Для нашего проекта:**

```sql
-- B-tree (по умолчанию)
CREATE UNIQUE INDEX idx_links_short_code ON links(short_code);
CREATE INDEX idx_links_original_url ON links(original_url);
```

**Почему B-tree:**
- `short_code` — уникальные значения, точное совпадение (`WHERE short_code = 'abc123'`)
- `original_url` — поиск дубликатов (`WHERE original_url = 'https://...'`)
- B-tree — универсальный выбор, Hash не даёт преимуществ

**Когда Hash лучше:**
- Только точное совпадение
- Очень большая таблица (>100M строк)
- Hash-индекс занимает меньше места

**Пример использования Hash:**
```sql
CREATE INDEX idx_sessions_token ON sessions USING HASH (token);
-- Только для поиска по токену (точный match)
```

---

### 9. Когда индекс вреден?

**Ответ:**

**Индексы замедляют запись:**

| Операция | Без индекса | С 1 индексом | С 3 индексами |
|----------|-------------|--------------|---------------|
| `INSERT` | 1 мс | 2 мс | 4 мс |
| `UPDATE` | 1 мс | 2 мс | 4 мс |
| `DELETE` | 1 мс | 2 мс | 4 мс |

**Почему:**
- При `INSERT` нужно добавить строку в таблицу **и** в индекс
- При `UPDATE` нужно обновить индекс (если изменился индексируемый столбец)
- Каждый индекс — дополнительная структура данных на диске

**Когда индекс вреден:**

1. **Низкая селективность:**
   ```sql
   -- Плохо: индекс на столбец с 2 значениями
   CREATE INDEX idx_users_active ON users(is_active);  -- true/false
   -- Sequential Scan быстрее, чем Index Scan
   ```

2. **Частые записи, редкие чтения:**
   ```sql
   -- Таблица логов: 1000 INSERT/сек, 1 SELECT/день
   CREATE INDEX idx_logs_timestamp ON logs(created_at);  -- Замедляет INSERT
   ```

3. **Маленькие таблицы:**
   ```sql
   -- Таблица с 100 строками
   CREATE INDEX idx_settings_key ON settings(key);  -- Sequential Scan быстрее
   ```

4. **Широкие индексы:**
   ```sql
   -- Плохо: индекс на TEXT столбец
   CREATE INDEX idx_articles_content ON articles(content);  -- Огромный индекс
   ```

5. **Избыточные индексы:**
   ```sql
   CREATE INDEX idx_a ON links(short_code);
   CREATE INDEX idx_b ON links(short_code, original_url);  -- idx_a не нужен
   ```

**Для нашего проекта:**

```sql
-- ✅ Хорошие индексы:
CREATE UNIQUE INDEX idx_links_short_code ON links(short_code);
-- Селективность: 100% (уникальные значения)
-- Использование: каждый запрос GET /{short}

CREATE INDEX idx_links_original_url ON links(original_url);
-- Селективность: высокая (URL уникальны)
-- Использование: проверка дубликатов при создании
```

**Правило большого пальца:**
- Индекс полезен, если ускоряет запросы в 10+ раз
- Индекс вреден, если замедляет записи на 10%+
- Проверяйте через `EXPLAIN ANALYZE`

---

## Go-специфика

### 10. Почему `context.Context` передаётся первым параметром?

**Ответ:**

**Конвенция Go:** `context.Context` всегда первый параметр функции.

```go
// ✅ Правильно
func (r *LinkRepository) Create(ctx context.Context, link *model.Link) error

// ❌ Неправильно
func (r *LinkRepository) Create(link *model.Link, ctx context.Context) error
```

**Причины:**

1. **Единообразие:** Все Go-библиотеки следуют этой конвенции
2. **Читаемость:** Контекст сразу виден в сигнатуре
3. **Инструменты:** `go vet` проверяет порядок параметров

**Что такое `context.Context`:**

```go
type Context interface {
    Deadline() (deadline time.Time, ok bool)  // Таймаут
    Done() <-chan struct{}                     // Сигнал отмены
    Err() error                                // Причина отмены
    Value(key any) any                         // Значения (request ID, user)
}
```

**Использование:**

```go
// Таймаут
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

result, err := repo.GetByCode(ctx, "abc123")
if err == context.DeadlineExceeded {
    // Таймаут
}

// Отмена
ctx, cancel := context.WithCancel(context.Background())
go func() {
    time.Sleep(1 * time.Second)
    cancel()  // Отменяем все операции с этим контекстом
}()

// Значения (для request ID)
ctx = context.WithValue(ctx, RequestIDKey, "12345")
requestID := ctx.Value(RequestIDKey).(string)
```

**Почему не глобальная переменная:**
- Глобальные переменные усложняют тестирование
- Контекст явно передаётся через цепочку вызовов
- Можно отменить конкретную операцию, не затрагивая другие

---

### 11. Почему `pgxpool` вместо `pgx.Conn`?

**Ответ:**

**`pgx.Conn`** — одно соединение с БД:

```go
conn, err := pgx.Connect(ctx, databaseURL)
// Только одно соединение, не потокобезопасно
```

**Проблемы:**
- Не подходит для конкурентных запросов (HTTP-сервер)
- Нужно вручную управлять соединениями
- Нет пула соединений

**`pgxpool.Pool`** — пул соединений:

```go
pool, err := pgxpool.New(ctx, databaseURL)
// Пул из N соединений, потокобезопасно
```

**Преимущества:**

| Функция | `pgx.Conn` | `pgxpool.Pool` |
|---------|------------|----------------|
| **Конкурентность** | ❌ Одно соединение | ✅ Много соединений |
| **Потокобезопасность** | ❌ Нет | ✅ Да |
| **Переиспользование** | ❌ Вручную | ✅ Автоматически |
| **Настройка** | Минимальная | `MaxConns`, `MinConns`, таймауты |

**Как работает пул:**

```go
pool, _ := pgxpool.NewWithConfig(ctx, config)
config.MaxConns = 10  // Максимум 10 соединений
config.MinConns = 2   // Минимум 2 (тёплые)

// Запрос 1
conn1, _ := pool.Acquire(ctx)  // Берём соединение из пула
defer conn1.Release()          // Возвращаем в пул

// Запрос 2 (параллельно)
conn2, _ := pool.Acquire(ctx)  // Берём другое соединение
defer conn2.Release()

// Если все 10 соединений заняты — ждём
```

**Для HTTP-сервера:**
- Каждый запрос может выполняться в отдельной goroutine
- Пул автоматически выделяет соединения
- Не нужно вручную управлять `conn.Close()`

---

## Итого

**Ключевые темы для собеседования:**

1. **REST vs gRPC** — когда что использовать
2. **HTTP 301 vs 302** — семантика редиректов
3. **Слоистая архитектура** — handler → service → repository
4. **Error wrapping** — `errors.Is` вместо `==`
5. **Dependency Injection** — интерфейсы и тестируемость
6. **Конкурентность** — race conditions, атомарные операции
7. **Индексы** — B-tree vs Hash, когда вредны
8. **Context** — таймауты, отмена, значения
9. **Connection pool** — почему пул, а не одно соединение

**Дополнительные вопросы для самопроверки:**
- Как бы вы реализовали rate limiting?
- Как масштабировать приложение горизонтально?
- Что такое eventual consistency и где она возникает в нашем проекте?
- Как бы вы реализовали TTL для ссылок?

---

# Собеседование по Этапу 2: Улучшения

> Вопросы по health-check, Prometheus-метрикам, Docker multi-stage build и Kubernetes.

---

## Docker

### 1. Что такое multi-stage build в Docker? Зачем он нужен?

**Ответ:**

**Multi-stage build** — способ сборки Docker-образа, при котором используется несколько `FROM` инструкций. Каждый stage может копировать артефакты из предыдущих stages.

**Без multi-stage (плохо):**

```dockerfile
FROM golang:1.24
WORKDIR /app
COPY . .
RUN go build -o /shortener ./cmd/api
EXPOSE 8080
CMD ["/shortener"]
```

**Проблемы:**
- Образ весит ~800MB (весь golang + исходники + зависимости)
- В образе остаются исходники, go.mod, тесты
- Уязвимости в инструментах сборки (gcc, git)

**С multi-stage (хорошо):**

```dockerfile
# Stage 1: сборка
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /shortener ./cmd/api

# Stage 2: runtime (только бинарник)
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /shortener .
CMD ["./shortener"]
```

**Результат:**
- Образ весит ~15-30MB (только alpine + бинарник)
- Нет исходников, зависимостей сборки
- Меньше уязвимостей (меньше поверхность атаки)

**Ключевые оптимизации:**

| Опция | Назначение |
|-------|-----------|
| `CGO_ENABLED=0` | Статический бинарник (не зависит от glibc) |
| `GOOS=linux` | Целевая ОС — Linux |
| `-ldflags="-s -w"` | Удаление debug-информации (-10-20% размера) |

**Для продакшена:**
- Можно использовать `scratch` (пустой образ) вместо `alpine` — образ ~10MB
- Или `distroless` (Google) — минимальный образ без shell

---

### 2. Разница между liveness probe и readiness probe

**Ответ:**

| Probe | Назначение | При падении |
|-------|-----------|-------------|
| **Liveness** | "Жив" ли процесс? | Pod перезапускается |
| **Readiness** | Готов принимать трафик? | Pod исключается из Service |

**Liveness probe:**

```yaml
livenessProbe:
  httpGet:
    path: /live
    port: 8080
  initialDelaySeconds: 15
  periodSeconds: 20
```

**Когда использовать:**
- Обнаружение deadlock, утечки памяти
- Приложение зависло, но процесс жив

**Readiness probe:**

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
```

**Когда использовать:**
- Проверка доступности БД, Redis, Kafka
- Загрузка кэша, инициализация данных
- Приложение не готово принимать трафик

**Пример сценария:**

```
1. Pod запускается
2. Readiness probe падает (БД недоступна) → трафик не идёт
3. БД восстанавливается
4. Readiness probe проходит → трафик идёт
5. Приложение падает (panic)
6. Liveness probe падает → pod перезапускается
```

**Наша реализация:**

Мы сделали `/health` как readiness probe — проверяет соединение с БД. Если БД недоступна:

```json
{
  "status": "error",
  "database": "error",
  "error": "connection refused"
}
```

HTTP 503 → Kubernetes исключит pod из Service.

---

### 3. Как работает graceful shutdown в Kubernetes?

**Ответ:**

Когда Kubernetes останавливает pod (scale down, update, node drain):

1. **SIGTERM** отправляется в PID 1 контейнера
2. **terminationGracePeriodSeconds** (по умолчанию 30с) — время на graceful shutdown
3. Если приложение не завершилось — **SIGKILL**

**Правильный graceful shutdown:**

```go
func main() {
    // 1. Запускаем сервер
    go func() {
        server.ListenAndServe()
    }()

    // 2. Ждём SIGTERM
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
    <-quit

    // 3. Graceful shutdown с таймаутом
    ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
    defer cancel()
    
    server.Shutdown(ctx)
    pool.Close()
}
```

**Почему 25 секунд (а не 30):**
- Kubernetes даёт 30 секунд (terminationGracePeriodSeconds)
- Оставляем 5 секунд на закрытие соединений БД, flush логов
- Если не успеем — SIGKILL

**Что происходит при Shutdown:**

1. Сервер перестаёт принимать новые соединения
2. Активные запросы дорабатываются
3. По истечении контекста — принудительное закрытие

**Важно:**
- Обработать **оба** сигнала: `SIGTERM` (от Kubernetes) и `SIGINT` (Ctrl+C при локальной разработке)
- PID 1 в контейнере должен быть ваше приложение (не shell)
- В Dockerfile: `CMD ["./shortener"]`, а не `CMD ./shortener`

---

## Observability (наблюдаемость)

### 4. Что такое Prometheus-метрики? Какие типы бывают?

**Ответ:**

**Prometheus** — система мониторинга с pull-моделью (сама забирает метрики по HTTP).

**Архитектура:**

```
Application → /metrics (HTTP) ← Prometheus (pull каждые 15с)
                                    ↓
                                Time Series DB
                                    ↓
                              Grafana (визуализация)
```

**Типы метрик:**

| Тип | Назначение | Пример |
|-----|-----------|--------|
| **Counter** | Монотонно растущий счётчик | `http_requests_total` |
| **Gauge** | Текущее значение (может уменьшаться) | `temperature`, `active_connections` |
| **Histogram** | Распределение значений по бакетам | `request_duration_seconds` |
| **Summary** | Квантили (p50, p90, p99) | `request_duration_seconds{quantile="0.99"}` |

**Counter (счётчик):**

```go
httpRequestsTotal := promauto.NewCounterVec(
    prometheus.CounterOpts{
        Name: "http_requests_total",
        Help: "Total number of HTTP requests",
    },
    []string{"method", "status"},
)

// Использование
httpRequestsTotal.WithLabelValues("GET", "200").Inc()
```

**Gauge (текущее значение):**

```go
activeConns := promauto.NewGaugeFunc(
    prometheus.GaugeOpts{
        Name: "db_pool_active_connections",
    },
    func() float64 {
        return float64(pool.Stat().TotalConns() - pool.Stat().IdleConns())
    },
)
```

**Histogram (распределение):**

```go
requestDuration := promauto.NewHistogramVec(
    prometheus.HistogramOpts{
        Name:    "http_request_duration_seconds",
        Buckets: prometheus.DefBuckets, // 0.005, 0.01, 0.025, ..., 10
    },
    []string{"method", "path"},
)

// Использование
requestDuration.WithLabelValues("GET", "/api").Observe(0.123)
```

**Наши метрики:**

```
http_requests_total{method="POST",path="/api/v1/links",status="201"} 42
http_request_duration_seconds_bucket{method="GET",path="/health",le="0.005"} 100
db_pool_active_connections 3
db_pool_idle_connections 7
db_pool_total_connections 10
```

**PromQL запросы:**

```promql
# RPS (requests per second)
rate(http_requests_total[5m])

# Error rate
sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m]))

# P99 latency
histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))
```

---

### 5. Почему middleware для метрик, а не ручные вызовы?

**Ответ:**

**Вариант 1: Ручные вызовы (плохо):**

```go
func (h *LinkHandler) CreateLink(w http.ResponseWriter, r *http.Request) {
    start := time.Now()
    
    // ... бизнес-логика ...
    
    metrics.RequestsTotal.WithLabelValues("POST", "201").Inc()
    metrics.Duration.WithLabelValues("POST").Observe(time.Since(start).Seconds())
}
```

**Проблемы:**
- Дублирование кода в каждом handler
- Легко забыть добавить метрику
- Сложно менять логику (например, добавить path)

**Вариант 2: Middleware (хорошо):**

```go
func Metrics(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        rw := newMetricsResponseWriter(w)  // перехватываем status code
        
        next.ServeHTTP(rw, r)
        
        metrics.RequestsTotal.WithLabelValues(
            r.Method, r.URL.Path, strconv.Itoa(rw.statusCode),
        ).Inc()
        metrics.Duration.WithLabelValues(
            r.Method, r.URL.Path,
        ).Observe(time.Since(start).Seconds())
    })
}

// Применяем ко всем handlers
handler := middleware.Metrics(mux)
```

**Преимущества:**
- Один раз написали — применяется ко всем endpoints
- Handler не знает о метриках (Single Responsibility)
- Легко отключить/заменить

**Паттерн: Chain of Responsibility**

```
Request → RequestID → Metrics → Logging → Handler → Response
```

Каждый middleware:
- Делает свою работу до handler
- Вызывает `next.ServeHTTP()`
- Делает свою работу после handler

---

## Kubernetes

### 6. Как Kubernetes использует health-check?

**Ответ:**

Kubernetes использует probes для управления жизненным циклом pod'ов.

**Жизненный цикл pod:**

```
Created → Running → Ready (readiness OK) → Serving traffic
                      ↓ (readiness fail)
                   Not Ready (removed from Service)
                      ↓ (liveness fail)
                   Restarted (new container)
```

**Пример Deployment:**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: url-shortener
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: api
        image: url-shortener:latest
        ports:
        - containerPort: 8080
        
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
          timeoutSeconds: 3
          failureThreshold: 3
        
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 15
          periodSeconds: 20
          timeoutSeconds: 3
          failureThreshold: 3
        
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "500m"
```

**Параметры probe:**

| Параметр | Описание |
|----------|----------|
| `initialDelaySeconds` | Задержка перед первым запросом |
| `periodSeconds` | Интервал между проверками |
| `timeoutSeconds` | Таймаут запроса |
| `failureThreshold` | Количество фейлов до действия |

**Типы probe:**

| Тип | Использование |
|-----|---------------|
| `httpGet` | HTTP-запрос (2xx/3xx = OK) |
| `tcpSocket` | TCP-соединение |
| `exec` | Выполнение команды в контейнере |

**Для нашего приложения:**
- `/health` — readiness probe (проверяет БД)
- Для liveness можно использовать тот же эндпоинт или простой `/live` (только процесс жив)

---

## Индексы PostgreSQL

### 7. Когда индекс вреден?

**Ответ:**

**Индексы замедляют запись:**

| Операция | Без индекса | С 1 индексом | С 3 индексами |
|----------|-------------|--------------|---------------|
| `INSERT` | 1 мс | 2 мс | 4 мс |
| `UPDATE` | 1 мс | 2 мс | 4 мс |
| `DELETE` | 1 мс | 2 мс | 4 мс |

**Когда индекс вреден:**

1. **Низкая селективность** — мало уникальных значений
   ```sql
   CREATE INDEX idx_active ON users(is_active);  -- только true/false
   -- Sequential Scan быстрее
   ```

2. **Частые записи, редкие чтения**
   ```sql
   CREATE INDEX idx_logs_time ON logs(created_at);  -- 1000 INSERT/сек
   -- Замедляет INSERT
   ```

3. **Маленькие таблицы**
   ```sql
   CREATE INDEX idx_settings ON settings(key);  -- 100 строк
   -- Sequential Scan быстрее
   ```

4. **Широкие индексы**
   ```sql
   CREATE INDEX idx_content ON articles(content);  -- TEXT столбец
   -- Огромный индекс
   ```

5. **Избыточные индексы**
   ```sql
   CREATE INDEX idx_a ON links(short_code);
   CREATE INDEX idx_b ON links(short_code, original_url);  -- idx_a не нужен
   ```

**Правило:**
- Индекс полезен, если ускоряет запросы в 10+ раз
- Индекс вреден, если замедляет записи на 10%+
- Проверяйте через `EXPLAIN ANALYZE`

---

### 8. B-tree vs Hash индексы

**Ответ:**

| Критерий | B-tree | Hash |
|----------|--------|------|
| **Точное совпадение** (`=`) | ✅ | ✅ (быстрее) |
| **Диапазоны** (`>`, `<`, `BETWEEN`) | ✅ | ❌ |
| **Сортировка** (`ORDER BY`) | ✅ | ❌ |
| **Размер на диске** | Больше | Меньше |
| **WAL (журнал)** | Полная поддержка | Ограниченная (PostgreSQL 10+) |

**Для нашего проекта:**

```sql
-- B-tree (по умолчанию)
CREATE UNIQUE INDEX idx_links_short_code ON links(short_code);
CREATE INDEX idx_links_original_url ON links(original_url);
```

**Почему B-tree:**
- Универсальный выбор
- Поддерживает все операции
- Hash не даёт преимуществ для наших запросов

**Когда Hash лучше:**
- Только точное совпадение
- Очень большая таблица (>100M строк)
- Hash-индекс занимает меньше места

---

## Итого

**Ключевые темы для собеседования:**

1. **Multi-stage build** — уменьшение размера Docker-образа
2. **Liveness vs Readiness** — управление жизненным циклом pod
3. **Graceful shutdown** — корректное завершение работы
4. **Prometheus-метрики** — Counter, Gauge, Histogram, Summary
5. **Middleware для метрик** — DRY, Single Responsibility
6. **Kubernetes probes** — автоматическое управление pod'ами
7. **Индексы** — когда вредны, B-tree vs Hash

**Дополнительные вопросы для самопроверки:**
- Как настроить алерты в Prometheus?
- Что такое Service Mesh (Istio, Linkerd)?
- Как работает HPA (Horizontal Pod Autoscaler)?
- Что такое distributed tracing (Jaeger, Zipkin)?

---

## Следующий этап

После изучения этих вопросов переходим к **Этапу 3: Кэш (Redis)**.