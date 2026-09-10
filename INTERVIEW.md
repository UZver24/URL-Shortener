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

---

# Собеседование по Этапу 3: Кэш (Redis)

> Документация с типичными вопросами на техническом собеседовании и подробными ответами по теме кэширования и Redis.

---

## Основы кэширования

### 1. Что такое кэширование и зачем оно нужно?

**Ответ:**

**Кэширование** — это временное хранение часто используемых данных в быстрой памяти (обычно in-memory) для ускорения доступа к ним.

**Аналогия из жизни:**
- **Без кэша:** Каждый раз, когда нужна книга, идёшь в библиотеку (БД)
- **С кэшем:** Держишь популярные книги на столе (Redis) — берёшь мгновенно

**Зачем нужно кэширование:**

| Проблема без кэша | Решение с кэшем |
|-------------------|-----------------|
| Высокая latency (50-200ms на запрос к БД) | Низкая latency (1-5ms из Redis) |
| Высокая нагрузка на БД (1000 RPS → 1000 запросов) | Снижение нагрузки (90% из кэша) |
| Дорогое масштабирование БД | Дешёвое масштабирование кэша |
| Ограниченная пропускная способность БД | Redis держит 100k+ RPS |

**Когда кэш эффективен:**
- **Read-heavy нагрузка:** 80%+ запросов — чтение (как в URL-сокращателе: `GET /{short}`)
- **Повторяющиеся данные:** Одни и те же ссылки запрашиваются многократно
- **Допустима eventual consistency:** Данные могут быть слегка устаревшими (TTL)

**Когда кэш НЕ нужен:**
- Write-heavy нагрузка (больше записей, чем чтений)
- Данные меняются чаще, чем читаются
- Требуется строгая consistency (банковские транзакции)

**Для нашего проекта:**
```
80% запросов → GET /{short} (чтение из БД)
↓
С кэшем: 90% из Redis (1ms), 10% из PostgreSQL (50ms)
↓
Средняя latency: 0.9*1 + 0.1*50 = 5.9ms (вместо 50ms)
```

**Follow-up вопрос:** А если данные изменились в БД, а в кэше старые?

**Ответ:** Это проблема **cache invalidation** (инвалидация кэша). Решения:
1. **TTL (Time To Live):** Данные устаревают автоматически через N секунд
2. **Инвалидация при изменении:** При `DELETE /links/{code}` удаляем из кэша
3. **Write-through:** Пишем одновременно в БД и кэш (сложнее, но consistent)

Мы используем **TTL + инвалидацию при удалении** — простой и эффективный подход.

---

### 2. Cache-aside vs Write-through vs Write-back: в чём разница?

**Ответ:**

Это три основных стратегии кэширования:

#### Cache-aside (Lazy Loading)

```
Чтение:
  1. Проверить кэш
  2. Если есть → вернуть из кэша (cache hit)
  3. Если нет → получить из БД (cache miss)
  4. Сохранить в кэш
  5. Вернуть данные

Запись:
  1. Записать в БД
  2. НЕ обновлять кэш (ленивое кэширование)
```

**Плюсы:**
- Простая реализация
- Кэш содержит только "горячие" данные (те, что запрашиваются)
- Устойчив к падению кэша (fallback к БД)

**Минусы:**
- Cache miss увеличивает latency (2 запроса: кэш + БД)
- Данные в кэше могут устаревать (решается TTL)

**Write-through**

```
Запись:
  1. Записать в кэш
  2. Записать в БД
  3. Вернуть успех

Чтение:
  1. Прочитать из кэша (всегда hit, если записано)
```

**Плюсы:**
- Данные в кэше всегда актуальны
- Быстрое чтение (всегда cache hit)

**Минусы:**
- Медленная запись (2 операции)
- Кэш может содержать "холодные" данные (которые редко читаются)
- Сложная обработка ошибок (что если кэш упал?)

**Write-back (Write-behind)**

```
Запись:
  1. Записать в кэш
  2. Вернуть успех (асинхронно)
  3. Периодически синхронизировать кэш → БД (batch)

Чтение:
  1. Прочитать из кэша
```

**Плюсы:**
- Очень быстрая запись (только кэш)
- Снижение нагрузки на БД (batch updates)

**Минусы:**
- Риск потери данных (если кэш упал до синхронизации)
- Сложная реализация (нужны воркеры, retry, dead letter queue)

**Сравнительная таблица:**

| Стратегия | Чтение | Запись | Consistency | Сложность | Когда использовать |
|-----------|--------|--------|-------------|-----------|-------------------|
| **Cache-aside** | Fast (hit) / Slow (miss) | Fast | Eventual | Низкая | Read-heavy, допустима eventual consistency |
| **Write-through** | Always fast | Slow | Strong | Средняя | Write + read, нужна consistency |
| **Write-back** | Always fast | Very fast | Weak | Высокая | Write-heavy, допустима потеря данных |

**Для нашего проекта:**

Мы выбрали **cache-aside** по нескольким причинам:
1. **Read-heavy:** 80%+ запросов — `GET /{short}`
2. **Простота:** Легко реализовать и отладить
3. **Graceful degradation:** Если Redis упал, работаем с PostgreSQL
4. **Экономия:** Кэш содержит только "горячие" ссылки

**Код из нашего проекта:**

```go
func (s *LinkService) GetOriginalURL(ctx context.Context, code string) (string, error) {
    // 1. Пробуем получить из кэша
    if s.cache != nil {
        link, err := s.cache.Get(ctx, code)
        if err == nil {
            // Cache hit — возвращаем из кэша
            return link.OriginalURL, nil
        }
        // Cache miss — продолжаем с БД
    }

    // 2. Получаем из БД
    link, err := s.repo.GetByCode(ctx, code)
    if err != nil {
        return "", err
    }

    // 3. Сохраняем в кэш (ленивое кэширование)
    if s.cache != nil {
        s.cache.Set(ctx, link)
    }

    return link.OriginalURL, nil
}
```

---

### 3. TTL и стратегии вытеснения (LRU, LFU, FIFO)

**Ответ:**

**TTL (Time To Live)** — время жизни записи в кэше. После истечения TTL данные автоматически удаляются.

**Зачем нужен TTL:**
1. **Инвалидация:** Данные устаревают автоматически
2. **Освобождение памяти:** Предотвращает переполнение кэша
3. **Consistency:** Баланс между скоростью и актуальностью

**Как выбрать TTL:**

| Тип данных | TTL | Пример |
|------------|-----|--------|
| Статические (не меняются) | Часы/дни | Конфигурация, справочники |
| Полустатические | Минуты/часы | Профили пользователей |
| Динамические | Секунды/минуты | Статистика, счётчики |
| Сессии | Минуты | JWT tokens, корзины |

**Для нашего проекта:**
```bash
REDIS_TTL=3600  # 1 час
```

Почему 1 час:
- Ссылки редко меняются (создал → используется месяцами)
- Статистика (`clicks`) может устаревать, но это не критично
- Баланс между hit ratio и актуальностью

#### Стратегии вытеснения (Eviction Policies)

Когда кэш заполнен, нужно решить, какие данные удалить:

**LRU (Least Recently Used)**

Удаляет данные, которые не использовались дольше всех.

```
Кэш (максимум 3 элемента):
  [A, B, C]
  
Запрос A → [A, B, C] (A обновлён)
Запрос D → [D, A, B] (C вытеснен, так как использовался давно)
```

**Плюсы:**
- Простая реализация
- Хорошо работает для большинства сценариев

**Минусы:**
- Не учитывает частоту использования

**LFU (Least Frequently Used)**

Удаляет данные, которые используются реже всего.

```
Кэш:
  A (10 запросов), B (5 запросов), C (1 запрос)
  
Запрос D → [A, B, D] (C вытеснен, так как использовался 1 раз)
```

**Плюсы:**
- Учитывает популярность данных
- Хорошо для "горячих" данных

**Минусы:**
- Сложная реализация (нужно считать частоту)
- Может удерживать старые популярные данные

**FIFO (First In, First Out)**

Удаляет данные в порядке добавления.

```
Кэш:
  [A (старый), B, C]
  
Запрос D → [B, C, D] (A вытеснен, так как добавлен первым)
```

**Плюсы:**
- Очень простая реализация

**Минусы:**
- Не учитывает использование
- Может вытеснить популярные данные

**Сравнительная таблица:**

| Стратегия | Реализация | Эффективность | Когда использовать |
|-----------|------------|---------------|-------------------|
| **LRU** | Средняя | Высокая | Универсальный выбор |
| **LFU** | Сложная | Очень высокая | Есть чёткие "горячие" данные |
| **FIFO** | Простая | Низкая | Временные данные (очереди) |
| **Random** | Очень простая | Низкая | Нет паттерна доступа |

**Redis eviction policies:**

```bash
# Настройка в redis.conf
maxmemory 1gb
maxmemory-policy allkeys-lru  # LRU для всех ключей
```

Доступные политики:
- `noeviction` — не вытеснять (возвращать ошибку при переполнении)
- `allkeys-lru` — LRU для всех ключей
- `volatile-lru` — LRU только для ключей с TTL
- `allkeys-lfu` — LFU для всех ключей
- `volatile-ttl` — удалять ключи с наименьшим TTL

**Для нашего проекта:**
```bash
maxmemory-policy allkeys-lru
```

Почему LRU:
- Простая и эффективная стратегия
- Автоматически вытесняет "холодные" ссылки
- Redis реализует её оптимально (approximated LRU)

---

### 4. Инвалидация кэша: почему это сложно?

**Ответ:**

> "There are only two hard things in Computer Science: cache invalidation and naming things." — Phil Karlton

**Инвалидация кэша** — это процесс удаления устаревших данных из кэша при изменении в БД.

**Почему это сложно:**

1. **Race conditions:**
   ```
   T1: Чтение из БД → [данные старые]
   T2: Запись в БД → [данные новые]
   T1: Запись в кэш → [кэш содержит старые данные]
   ```

2. **Distributed cache:**
   ```
   Сервер A: обновил БД, инвалидировал свой кэш
   Сервер B: кэш всё ещё содержит старые данные
   ```

3. **Partial failures:**
   ```
   Запись в БД: успех
   Инвалидация кэша: ошибка (Redis упал)
   Результат: кэш содержит stale data
   ```

**Паттерны инвалидации:**

#### 1. TTL-based (Time To Live)

Данные устаревают автоматически через N секунд.

```go
cache.Set(ctx, key, value, 1*time.Hour)  // TTL = 1 час
```

**Плюсы:**
- Простая реализация
- Не нужна явная инвалидация

**Минусы:**
- Данные могут устаревать до истечения TTL
- Сложно выбрать правильный TTL

#### 2. Write-through invalidation

При изменении данных обновляем и БД, и кэш.

```go
func (s *Service) UpdateLink(code string, newURL string) error {
    // 1. Обновить БД
    if err := s.repo.Update(code, newURL); err != nil {
        return err
    }
    
    // 2. Обновить кэш (или удалить)
    s.cache.Delete(ctx, code)
    
    return nil
}
```

**Плюсы:**
- Данные в кэше всегда актуальны

**Минусы:**
- Сложная обработка ошибок
- Увеличивает latency записи

#### 3. Event-driven invalidation

Используем message broker (Kafka, RabbitMQ) для уведомлений об изменениях.

```
Сервис A: обновил БД → опубликовал событие "link.updated"
Сервис B: подписался на события → инвалидирует кэш при получении
```

**Плюсы:**
- Работает в distributed системах
- Decoupled (сервисы не знают друг о друге)

**Минусы:**
- Сложная инфраструктура
- Возможна задержка (eventual consistency)

#### 4. Cache-aside с инвалидацией

Наш подход: кэш заполняется лениво, инвалидируется при изменении.

```go
func (s *Service) DeleteLink(code string) error {
    // 1. Удалить из БД
    if err := s.repo.Delete(code); err != nil {
        return err
    }
    
    // 2. Инвалидировать кэш
    s.cache.Delete(ctx, code)
    
    return nil
}
```

**Для нашего проекта:**

Мы используем **TTL + cache-aside invalidation**:
- **TTL = 1 час:** Данные устаревают автоматически
- **Инвалидация при удалении:** `DELETE /links/{code}` удаляет из кэша
- **Graceful degradation:** Если Redis упал, работаем с БД

---

## Redis vs Memcached

### 5. Redis vs Memcached: когда что использовать?

**Ответ:**

| Критерий | Redis | Memcached |
|----------|-------|-----------|
| **Тип** | In-memory database | In-memory cache |
| **Структуры данных** | String, Hash, List, Set, Sorted Set, Stream | Только String (key-value) |
| **Персистентность** | RDB snapshots, AOF logs | Нет (только in-memory) |
| **Репликация** | Master-slave, Sentinel, Cluster | Нет |
| **Транзакции** | MULTI/EXEC (atomic) | Нет |
| **Pub/Sub** | Есть | Нет |
| **Lua scripts** | Есть | Нет |
| **Производительность** | ~100k RPS | ~200k RPS (проще) |
| **Использование памяти** | Выше (metadata, структуры) | Ниже (только key-value) |

**Когда Redis:**
- Нужны сложные структуры данных (sorted sets для leaderboards)
- Нужна персистентность (не терять данные при перезапуске)
- Нужна репликация (high availability)
- Нужны транзакции (atomic operations)
- Нужен pub/sub (real-time уведомления)

**Когда Memcached:**
- Только простое key-value кэширование
- Максимальная производительность (2x быстрее Redis)
- Минимальное использование памяти
- Не нужна персистентность

**Для нашего проекта:**

Мы выбрали **Redis** по нескольким причинам:
1. **JSON-сериализация:** Храним `model.Link` как JSON string
2. **TTL:** Встроенная поддержка expiration
3. **Персистентность:** Можно настроить RDB snapshots (опционально)
4. **Экосистема:** Богатая библиотека клиентов для Go

**Пример из нашего кода:**

```go
// Сохранение ссылки в Redis
func (c *LinkCache) Set(ctx context.Context, link *model.Link) error {
    key := fmt.Sprintf("link:%s", link.ShortCode)
    
    data, err := json.Marshal(link)
    if err != nil {
        return err
    }
    
    // Set с TTL
    return c.client.Set(ctx, key, data, c.ttl).Err()
}

// Получение из Redis
func (c *LinkCache) Get(ctx context.Context, code string) (*model.Link, error) {
    key := fmt.Sprintf("link:%s", code)
    
    data, err := c.client.Get(ctx, key).Bytes()
    if err != nil {
        if errors.Is(err, redis.Nil) {
            return nil, model.ErrLinkNotFound  // Cache miss
        }
        return nil, err
    }
    
    var link model.Link
    if err := json.Unmarshal(data, &link); err != nil {
        return nil, err
    }
    
    return &link, nil
}
```

---

## Graceful Degradation

### 6. Что такое graceful degradation и зачем нужно?

**Ответ:**

**Graceful degradation** — это стратегия, при которой система продолжает работать (с ограниченной функциональностью) при недоступности одного из компонентов.

**Аналогия:**
- **Без graceful degradation:** Если сломался лифт, ты не можешь подняться на этаж
- **С graceful degradation:** Если сломался лифт, ты идёшь по лестнице (медленнее, но работает)

**Для кэша:**

```
Нормальная работа:
  Запрос → Redis (1ms) → Ответ
  
Redis упал:
  Запрос → Redis (ошибка) → PostgreSQL (50ms) → Ответ
```

**Зачем нужно:**
1. **Availability:** Система работает даже при partial failures
2. **Resilience:** Устойчивость к сбоям зависимостей
3. **User experience:** Пользователи не видят ошибок (только повышенная latency)

**Как реализовать:**

```go
func (s *LinkService) GetOriginalURL(ctx context.Context, code string) (string, error) {
    var link *model.Link
    var err error

    // 1. Пробуем кэш
    if s.cache != nil {
        link, err = s.cache.Get(ctx, code)
        if err == nil {
            // Cache hit
            return link.OriginalURL, nil
        }
        // Cache miss или ошибка — логируем, но продолжаем
        if !errors.Is(err, model.ErrLinkNotFound) {
            s.logger.Warn("cache error, falling back to DB", "error", err)
        }
    }

    // 2. Fallback к БД
    link, err = s.repo.GetByCode(ctx, code)
    if err != nil {
        return "", err
    }

    // 3. Пытаемся сохранить в кэш (если доступен)
    if s.cache != nil {
        if err := s.cache.Set(ctx, link); err != nil {
            s.logger.Warn("failed to cache", "error", err)
            // Не возвращаем ошибку — это не критично
        }
    }

    return link.OriginalURL, nil
}
```

**Ключевые моменты:**
1. **Проверка nil:** `if s.cache != nil` — кэш может быть отключён
2. **Обработка ошибок:** Логируем, но не прерываем выполнение
3. **Fallback:** Всегда есть запасной путь (БД)
4. **Не критичные ошибки:** Ошибка кэша не влияет на ответ пользователю

**Из нашего `main.go`:**

```go
// Подключаемся к Redis (graceful degradation)
redisClient, err := redisrepo.NewClient(cfg.RedisAddr(), ...)
if err != nil {
    logger.Warn("failed to connect to Redis, running without cache", "error", err)
    redisClient = nil  // Работаем без кэша
} else {
    linkCache = redisrepo.NewLinkCache(redisClient.GetClient(), logger, ttl)
}

// Передаём в сервис (cache может быть nil)
linkService := service.NewLinkService(linkRepo, logger, linkCache)
```

**Тестирование graceful degradation:**

```go
func TestCacheAside_CacheError_FallbackToDB(t *testing.T) {
    repo := newMockRepository()
    cache := newMockCache()
    cache.getErr = errors.New("redis connection error")  // Имитируем ошибку
    svc := NewLinkService(repo, testLogger(), cache)

    // Создаём ссылку в БД
    repo.links["fallback"] = &model.Link{
        ID: 1, ShortCode: "fallback", OriginalURL: "https://fallback.com",
    }

    // Получаем URL (кэш сломан, должен fallback к БД)
    url, err := svc.GetOriginalURL(context.Background(), "fallback")
    if err != nil {
        t.Fatalf("expected no error (fallback to DB), got %v", err)
    }

    if url != "https://fallback.com" {
        t.Errorf("expected 'https://fallback.com', got '%s'", url)
    }
}
```

---

## Метрики для кэша

### 7. Как измерять эффективность кэша (hit ratio)?

**Ответ:**

**Hit ratio** — это процент запросов, обслуженных из кэша.

```
Hit Ratio = Cache Hits / (Cache Hits + Cache Misses) * 100%
```

**Пример:**
```
За 1 час:
  Cache hits: 9000
  Cache misses: 1000
  
Hit ratio = 9000 / (9000 + 1000) * 100% = 90%
```

**Что считается хорошим hit ratio:**

| Hit Ratio | Оценка | Действия |
|-----------|--------|----------|
| **>95%** | Отлично | Кэш работает оптимально |
| **80-95%** | Хорошо | Можно оптимизировать TTL |
| **50-80%** | Средне | Проверить паттерн доступа |
| **<50%** | Плохо | Кэш неэффективен, пересмотреть стратегию |

**Метрики для кэша:**

```go
// Prometheus метрики
var (
    cacheHitsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "cache_hits_total",
            Help: "Total number of cache hits",
        },
        []string{"operation"},  // get, set, delete
    )

    cacheMissesTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "cache_misses_total",
            Help: "Total number of cache misses",
        },
        []string{"operation"},
    )

    cacheErrorsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "cache_errors_total",
            Help: "Total number of cache errors",
        },
        []string{"operation"},
    )
)
```

**Запись метрик:**

```go
func (s *LinkService) GetOriginalURL(ctx context.Context, code string) (string, error) {
    if s.cache != nil {
        link, err := s.cache.Get(ctx, code)
        if err == nil {
            middleware.RecordCacheHit("get")
            return link.OriginalURL, nil
        }
        if errors.Is(err, model.ErrLinkNotFound) {
            middleware.RecordCacheMiss("get")
        } else {
            middleware.RecordCacheError("get")
        }
    }
    // ...
}
```

**PromQL запросы:**

```promql
# Hit ratio за последние 5 минут
rate(cache_hits_total[5m]) / (rate(cache_hits_total[5m]) + rate(cache_misses_total[5m]))

# Количество ошибок кэша
rate(cache_errors_total[5m])

# Latency кэша
histogram_quantile(0.95, rate(cache_duration_seconds_bucket[5m]))
```

**Для нашего проекта:**

Целевой hit ratio: **80-90%**

Почему не 95%+:
- Новые ссылки ещё не в кэше (cold start)
- TTL = 1 час (данные устаревают)
- Часть запросов — уникальные ссылки

**Как улучшить hit ratio:**
1. **Увеличить TTL:** 1 час → 24 часа (но данные устаревают дольше)
2. **Pre-warming:** Загружать популярные ссылки в кэш при старте
3. **Больше памяти:** Увеличить `maxmemory` в Redis

---

## Health-check с Redis

### 8. Как обновить health-check для Redis?

**Ответ:**

**Health-check** должен проверять все критичные зависимости:
- PostgreSQL (критично — без БД не работаем)
- Redis (не критично — есть graceful degradation)

**Из нашего `health_handler.go`:**

```go
type HealthHandler struct {
    pool  *pgxpool.Pool
    redis RedisPinger  // интерфейс, может быть nil
}

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
    defer cancel()

    // Проверяем PostgreSQL
    dbStatus := "ok"
    if err := h.pool.Ping(ctx); err != nil {
        dbStatus = "error"
    }

    // Проверяем Redis (если доступен)
    cacheStatus := ""
    if h.redis != nil {
        cacheStatus = "ok"
        if err := h.redis.Ping(ctx); err != nil {
            cacheStatus = "error"
        }
    }

    // Формируем ответ
    status := "ok"
    httpStatus := http.StatusOK

    if dbStatus != "ok" {
        // БД критична — 503
        status = "error"
        httpStatus = http.StatusServiceUnavailable
    }

    resp := HealthResponse{
        Status:   status,
        Database: dbStatus,
        Cache:    cacheStatus,  // "ok", "error", или "" (если Redis отключён)
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(httpStatus)
    json.NewEncoder(w).Encode(resp)
}
```

**Примеры ответов:**

```bash
# Всё работает
curl http://localhost:8080/health
{"status":"ok","database":"ok","cache":"ok"}

# Redis упал (graceful degradation)
curl http://localhost:8080/health
{"status":"ok","database":"ok","cache":"error"}
# HTTP 200 — приложение работает

# PostgreSQL упал
curl http://localhost:8080/health
{"status":"error","database":"error","cache":"ok"}
# HTTP 503 — приложение не работает
```

**Почему Redis error → HTTP 200:**
- Redis не критичен (есть fallback к БД)
- Приложение продолжает работать
- Kubernetes не перезапустит pod (liveness probe проходит)

**Почему PostgreSQL error → HTTP 503:**
- БД критична (без неё не работаем)
- Kubernetes перезапустит pod (readiness probe fails)

---

## Docker Compose с Redis

### 9. Как добавить Redis в docker-compose?

**Ответ:**

**Из нашего `docker-compose.yml`:**

```yaml
services:
  postgres:
    image: postgres:16-alpine
    # ...

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

  api:
    build: .
    environment:
      REDIS_HOST: redis
      REDIS_PORT: 6379
      REDIS_PASSWORD: ""
      REDIS_DB: 0
      REDIS_TTL: 3600
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy

volumes:
  postgres_data:
  redis_data:
```

**Ключевые моменты:**

1. **Healthcheck:** `redis-cli ping` возвращает `PONG` если Redis работает
2. **Volume:** `redis_data:/data` для персистентности (опционально)
3. **depends_on:** API ждёт, пока Redis будет healthy
4. **Environment:** Конфигурация через переменные окружения

**Запуск:**

```bash
docker-compose up -d
docker-compose ps
# Проверяем, что все сервисы healthy

docker-compose logs redis
# Смотрим логи Redis
```

---

## Итого

**Ключевые темы для собеседования:**

1. **Кэширование** — зачем нужно, когда эффективно, паттерны доступа
2. **Cache-aside** — ленивое кэширование, простая реализация, graceful degradation
3. **TTL** — время жизни данных, баланс между hit ratio и актуальностью
4. **Инвалидация** — почему сложно, паттерны (TTL, write-through, event-driven)
5. **Redis vs Memcached** — когда что использовать, структуры данных
6. **Graceful degradation** — устойчивость к сбоям, fallback к БД
7. **Метрики** — hit ratio, hits/misses/errors, monitoring
8. **Health-check** — проверка зависимостей, HTTP 200 vs 503

**Дополнительные вопросы для самопроверки:**
- Что такое cache stampede (thundering herd) и как его избежать?
- Как работает Redis Cluster (sharding, replication)?
- Что такое Redis Pipeline и когда использовать?
- Как выбрать правильный TTL для разных типов данных?
- Что такое distributed cache и как синхронизировать между серверами?

---

## Следующий этап

После изучения этих вопросов переходим к **Этапу 4: Конкурентность** (воркер-пул для асинхронной записи счётчиков).

---

# Собеседование по Этапу 4: Конкурентность

> Документация с типичными вопросами на техническом собеседовании и подробными ответами по теме конкурентности в Go, воркер-пулов и graceful shutdown.

---

## Основы конкурентности в Go

### 1. Что такое конкурентность в Go? Goroutines vs threads

**Ответ:**

**Конкурентность** — это способность программы выполнять несколько задач одновременно (или псевдо-одновременно).

**Goroutine vs OS Thread:**

| Критерий | Goroutine | OS Thread |
|----------|-----------|-----------|
| **Размер стека** | 2KB (динамический) | 1-8MB (фиксированный) |
| **Создание** | Дешёвое (~300ns) | Дорогое (~1μs) |
| **Переключение** | M:N scheduler (Go runtime) | Kernel scheduler |
| **Количество** | Миллионы | Тысячи |
| **Управление** | Go runtime | ОС |
| **Блокировка** | Кооперативная | Вытесняющая |

**Почему goroutines эффективны:**

1. **Маленький стек:** 2KB vs 1MB — можно создать миллионы goroutines
2. **M:N scheduler:** Go runtime мультиплексирует N goroutines на M OS threads
3. **Work stealing:** Свободные threads забирают задачи у занятых
4. **Кооперативная многозадачность:** Переключение только в safe points (channel ops, function calls)

**Пример:**

```go
// OS threads (тяжело)
for i := 0; i < 10000; i++ {
    thread_create(...)  // ~1μs, 1MB стек
}

// Goroutines (легко)
for i := 0; i < 1000000; i++ {
    go func() { ... }()  // ~300ns, 2KB стек
}
```

**Когда использовать goroutines:**
- I/O-bound задачи (HTTP requests, DB queries, file operations)
- Параллельная обработка данных
- Event-driven системы

**Когда НЕ использовать:**
- CPU-bound задачи (math, cryptography) — используйте worker pool
- Когда нужен строгий контроль над threads (real-time systems)

**Для нашего проекта:**

Мы используем goroutines для асинхронного обновления счётчика:
```go
go func() {
    s.repo.IncrementClicks(bgCtx, link.ID)
}()
```

Но это создаёт неограниченное количество goroutines при высокой нагрузке. Решение — **worker pool** (см. вопрос 3).

---

### 2. Каналы: буферизированные vs небуферизированные

**Ответ:**

**Канал (channel)** — это типизированный конвейер для коммуникации между goroutines.

**Небуферизированные каналы:**

```go
ch := make(chan int)  // или make(chan int, 0)
```

**Поведение:**
- Отправка блокируется, пока нет получателя
- Получение блокируется, пока нет отправителя
- **Синхронная коммуникация** (rendezvous)

**Пример:**

```go
ch := make(chan int)

go func() {
    fmt.Println("Sending...")
    ch <- 42  // блокируется, пока main не прочитает
    fmt.Println("Sent")
}()

time.Sleep(1 * time.Second)
fmt.Println("Receiving...")
value := <-ch  // разблокирует sender
fmt.Println(value)
```

Вывод:
```
Sending...
(пауза 1 секунда)
Receiving...
Sent
42
```

**Буферизированные каналы:**

```go
ch := make(chan int, 10)  // буфер на 10 элементов
```

**Поведение:**
- Отправка блокируется, если буфер полон
- Получение блокируется, если буфер пуст
- **Асинхронная коммуникация** (до заполнения буфера)

**Пример:**

```go
ch := make(chan int, 3)

ch <- 1  // не блокируется (буфер пуст)
ch <- 2  // не блокируется
ch <- 3  // не блокируется (буфер полон)
ch <- 4  // БЛОКИРУЕТСЯ (буфер полон, нет получателя)
```

**Сравнительная таблица:**

| Критерий | Небуферизированный | Буферизированный |
|----------|-------------------|------------------|
| **Синхронизация** | Строгая | Слабая (до заполнения буфера) |
| **Производительность** | Ниже (всегда блокировка) | Выше (если буфер не полон) |
| **Использование** | Синхронизация, события | Очереди, batch processing |
| **Deadlock риск** | Высокий (если нет пары) | Низкий (если буфер достаточен) |

**Когда использовать небуферизированные:**
- Синхронизация goroutines (ping-pong, barrier)
- Event notifications (сигнал о завершении)
- Когда нужна строгая синхронизация

**Когда использовать буферизированные:**
- Producer-consumer паттерн
- Worker pools (очередь задач)
- Batch processing
- Когда producer быстрее consumer

**Для нашего проекта:**

Мы используем **буферизированный канал** для worker pool:
```go
type WorkerPool struct {
    tasks chan Task  // буферизированный канал задач
}

func NewWorkerPool(workers, bufferSize int) *WorkerPool {
    return &WorkerPool{
        tasks: make(chan Task, bufferSize),  // буфер на bufferSize задач
    }
}
```

Почему буферизированный:
- HTTP handlers не блокируются (если буфер не полон)
- Сглаживает пики нагрузки (burst traffic)
- Producer (HTTP handler) быстрее consumer (worker)

---

## Worker Pool Pattern

### 3. Паттерн Worker Pool: зачем нужен, как реализовать?

**Ответ:**

**Worker Pool** — это паттерн, при котором фиксированное количество воркеров (goroutines) обрабатывают задачи из общей очереди.

**Зачем нужен:**

| Проблема без worker pool | Решение с worker pool |
|--------------------------|----------------------|
| Неограниченное количество goroutines | Фиксированное число воркеров |
| High memory usage (миллионы goroutines) | Контролируемое потребление памяти |
| Нет контроля над concurrency | Ограниченная параллельность |
| Сложно graceful shutdown | Простой graceful shutdown |
| Нет retry при ошибках | Exponential backoff в worker |

**Пример из нашего проекта:**

**Без worker pool (плохо):**
```go
func (s *LinkService) GetOriginalURL(ctx context.Context, code string) (string, error) {
    link, err := s.repo.GetByCode(ctx, code)
    if err != nil {
        return "", err
    }
    
    // Проблема: создаём goroutine на каждый запрос
    go func() {
        s.repo.IncrementClicks(bgCtx, link.ID)
    }()
    
    return link.OriginalURL, nil
}
```

При 1000 RPS: 1000 goroutines/сек × 60 сек = 60,000 goroutines

**С worker pool (хорошо):**
```go
func (s *LinkService) GetOriginalURL(ctx context.Context, code string) (string, error) {
    link, err := s.repo.GetByCode(ctx, code)
    if err != nil {
        return "", err
    }
    
    // Решение: отправляем задачу в worker pool
    task := worker.Task{LinkID: link.ID}
    s.pool.Submit(task)  // не блокируется (если буфер не полон)
    
    return link.OriginalURL, nil
}
```

При 1000 RPS: 5 воркеров × 1000 задач в буфере = максимум 1005 goroutines

**Реализация Worker Pool:**

```go
type WorkerPool struct {
    tasks      chan Task       // буферизированный канал задач
    handler    TaskHandler     // функция-обработчик
    workers    int             // количество воркеров
    wg         sync.WaitGroup  // для graceful shutdown
    logger     *slog.Logger
    maxRetries int             // для exponential backoff
    baseDelay  time.Duration
}

func NewWorkerPool(cfg WorkerPoolConfig) *WorkerPool {
    return &WorkerPool{
        tasks:   make(chan Task, cfg.BufferSize),
        handler: cfg.Handler,
        workers: cfg.Workers,
        // ...
    }
}

func (p *WorkerPool) Start() {
    for i := 0; i < p.workers; i++ {
        p.wg.Add(1)
        go p.worker(i)
    }
}

func (p *WorkerPool) worker(id int) {
    defer p.wg.Done()
    
    for task := range p.tasks {  // читаем из канала
        p.processTask(id, task)  // обрабатываем задачу
    }
}

func (p *WorkerPool) Submit(task Task) error {
    select {
    case p.tasks <- task:  // non-blocking send
        return nil
    default:
        return ErrQueueFull  // буфер полон
    }
}

func (p *WorkerPool) Stop() error {
    close(p.tasks)  // закрываем канал — воркеры выйдут из for range
    p.wg.Wait()     // ждём завершения всех воркеров
    return nil
}
```

**Ключевые моменты:**

1. **Фиксированное число воркеров:** Контроль над concurrency
2. **Буферизированный канал:** Сглаживает пики нагрузки
3. **Non-blocking submit:** HTTP handlers не блокируются
4. **Graceful shutdown:** `close(tasks)` + `wg.Wait()`
5. **Error handling:** Exponential backoff в `processTask`

---

### 4. `sync.WaitGroup`: для чего используется?

**Ответ:**

**WaitGroup** — это примитив синхронизации для ожидания завершения группы goroutines.

**Методы:**

| Метод | Описание |
|-------|----------|
| `Add(delta int)` | Увеличивает счётчик на delta |
| `Done()` | Уменьшает счётчик на 1 (вызывается в goroutine) |
| `Wait()` | Блокируется, пока счётчик не станет 0 |

**Пример:**

```go
var wg sync.WaitGroup

for i := 0; i < 5; i++ {
    wg.Add(1)  // увеличиваем счётчик
    go func(id int) {
        defer wg.Done()  // уменьшаем счётчик при завершении
        fmt.Printf("Worker %d started\n", id)
        time.Sleep(1 * time.Second)
        fmt.Printf("Worker %d done\n", id)
    }(i)
}

fmt.Println("Waiting for workers...")
wg.Wait()  // блокируется, пока все 5 goroutines не завершатся
fmt.Println("All workers done")
```

**Почему `defer wg.Done()`:**
- Гарантирует, что счётчик уменьшится даже при panic
- Читаемость: явно видно, что goroutine "регистрируется"

**Для нашего проекта:**

Мы используем WaitGroup для graceful shutdown worker pool:

```go
func (p *WorkerPool) Start() {
    for i := 0; i < p.workers; i++ {
        p.wg.Add(1)  // регистрируем воркер
        go p.worker(i)
    }
}

func (p *WorkerPool) worker(id int) {
    defer p.wg.Done()  // уменьшаем счётчик при завершении
    
    for task := range p.tasks {
        p.processTask(id, task)
    }
}

func (p *WorkerPool) Stop() error {
    close(p.tasks)  // закрываем канал — воркеры выйдут из for range
    p.wg.Wait()     // ждём, пока все воркеры завершатся
    return nil
}
```

**Порядок graceful shutdown:**
1. `close(tasks)` — воркеры перестают получать новые задачи
2. Воркеры завершают текущие задачи и выходят из `for range`
3. `defer wg.Done()` уменьшает счётчик
4. `wg.Wait()` разблокируется, когда все воркеры завершатся

---

## Graceful Shutdown

### 5. Graceful shutdown: как правильно остановить воркеры?

**Ответ:**

**Graceful shutdown** — это корректное завершение работы, при котором:
1. Не принимаются новые запросы
2. Активные запросы завершаются
3. Очередь задач обрабатывается
4. Соединения закрываются

**Порядок graceful shutdown:**

```
1. Получить сигнал (SIGTERM/SIGINT)
   ↓
2. Остановить HTTP-сервер (не принимать новые запросы)
   ↓
3. Остановить worker pool (обработать оставшиеся задачи)
   ↓
4. Закрыть Redis
   ↓
5. Закрыть PostgreSQL
```

**Из нашего `main.go`:**

```go
// Graceful shutdown
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
sig := <-quit

logger.Info("received shutdown signal", "signal", sig.String())

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// 1. Останавливаем HTTP-сервер
if err := server.Shutdown(ctx); err != nil {
    logger.Error("server forced to shutdown", "error", err)
}

// 2. Останавливаем worker pool (обрабатываем оставшиеся задачи)
if err := workerPool.Stop(); err != nil {
    logger.Error("worker pool shutdown failed", "error", err)
}

// 3. Закрываем Redis
if redisClient != nil {
    redisClient.Close()
}

// 4. PostgreSQL закрывается через defer pool.Close()

logger.Info("server stopped gracefully")
```

**Почему такой порядок:**

1. **HTTP server first:** Перестаём принимать новые запросы
2. **Worker pool second:** Обрабатываем оставшиеся задачи (они могут использовать БД)
3. **Redis/PostgreSQL last:** Закрываем соединения после завершения всех операций

**Что если поменять порядок:**

```go
// НЕПРАВИЛЬНО: закрываем БД до worker pool
pool.Close()  // PostgreSQL закрыт
workerPool.Stop()  // воркеры пытаются использовать БД → ошибки
```

**Worker pool shutdown:**

```go
func (p *WorkerPool) Stop() error {
    close(p.tasks)  // закрываем канал задач
    
    // Воркеры:
    // for task := range p.tasks { ... }
    // range выходит, когда канал закрыт и пуст
    
    p.wg.Wait()  // ждём завершения всех воркеров
    return nil
}
```

**Почему `close(tasks)` работает:**
- `for range` читает из канала, пока он не закрыт И пуст
- После `close` воркеры обработают оставшиеся задачи и выйдут
- Это гарантирует, что все задачи из буфера будут обработаны

---

## Exponential Backoff

### 6. Exponential backoff: что это, зачем нужно?

**Ответ:**

**Exponential backoff** — это стратегия повторных попыток с увеличивающейся задержкой.

**Зачем нужно:**
1. **Избежать thundering herd:** Все клиенты не retry одновременно
2. **Дать системе восстановиться:** Увеличивающаяся задержка
3. **Снизить нагрузку:** Меньше retry при длительных сбоях

**Формула:**

```
delay = baseDelay * 2^attempt
```

**Пример (baseDelay = 1s):**

| Попытка | Задержка | Общее время |
|---------|----------|-------------|
| 1 | 1s | 1s |
| 2 | 2s | 3s |
| 3 | 4s | 7s |
| 4 | 8s | 15s |
| 5 | 16s | 31s |

**Из нашего проекта:**

```go
func (p *WorkerPool) processTask(workerID int, task Task) {
    var lastErr error
    for attempt := 0; attempt < p.maxRetries; attempt++ {
        err := p.handler(ctx, task)
        if err == nil {
            return  // успех
        }
        
        lastErr = err
        p.logger.Warn("task failed, retrying", "attempt", attempt+1, "error", err)
        
        // Exponential backoff: 1s, 2s, 4s, 8s, 16s
        if attempt < p.maxRetries-1 {
            delay := p.baseDelay * time.Duration(1<<uint(attempt))
            if delay > 16*time.Second {
                delay = 16 * time.Second  // cap
            }
            time.Sleep(delay)
        }
    }
    
    // Все попытки провалились
    p.logger.Error("task failed after all retries", "error", lastErr)
}
```

**Когда использовать:**
- Временные ошибки (network timeout, DB unavailable)
- Rate limiting (HTTP 429)
- Distributed systems (eventual consistency)

**Когда НЕ использовать:**
- Постоянные ошибки (invalid input, not found)
- Когда нужна быстрая реакция (real-time systems)
- Когда retry может ухудшить ситуацию (write operations без idempotency)

**Улучшения:**

1. **Jitter (случайность):**
   ```go
   delay := baseDelay * 2^attempt + rand(0, baseDelay)
   ```
   Предотвращает синхронизацию retry

2. **Circuit breaker:**
   ```go
   if failures > threshold {
       return ErrCircuitOpen  // не retry
   }
   ```
   Прекращает retry при длительных сбоях

---

## Batch Processing

### 7. Batch-обработки: преимущества, как реализовать?

**Ответ:**

**Batch processing** — это группировка нескольких операций в одну для снижения нагрузки на систему.

**Преимущества:**

| Критерий | Single | Batch (100) |
|----------|--------|-------------|
| **DB queries** | 100 | 1 |
| **Network roundtrips** | 100 | 1 |
| **Latency** | 100 × 5ms = 500ms | 1 × 10ms = 10ms |
| **DB load** | Высокая | Низкая |

**SQL batch update:**

```sql
-- Single update (100 раз)
UPDATE links SET clicks = clicks + 1 WHERE id = 1;
UPDATE links SET clicks = clicks + 1 WHERE id = 2;
...

-- Batch update (1 раз)
UPDATE links
SET clicks = clicks + batch.count
FROM (VALUES (1, 5), (2, 3), (3, 10)) AS batch(id, count)
WHERE links.id = batch.id;
```

**Реализация в worker pool:**

```go
type BatchingWorker struct {
    tasks     chan Task
    batch     []Task
    batchSize int
    flushInterval time.Duration
    ticker    *time.Ticker
}

func (w *BatchingWorker) Run() {
    w.ticker = time.NewTicker(w.flushInterval)
    defer w.ticker.Stop()
    
    for {
        select {
        case task := <-w.tasks:
            w.batch = append(w.batch, task)
            if len(w.batch) >= w.batchSize {
                w.flush()
            }
        case <-w.ticker.C:
            if len(w.batch) > 0 {
                w.flush()
            }
        }
    }
}

func (w *BatchingWorker) flush() {
    // Batch UPDATE
    query := `
        UPDATE links
        SET clicks = clicks + batch.count
        FROM (VALUES ...) AS batch(id, count)
        WHERE links.id = batch.id
    `
    w.db.Exec(query, w.batch)
    w.batch = w.batch[:0]  // очищаем batch
}
```

**Когда использовать:**
- Write-heavy нагрузка
- Batch операции допустимы (не нужна immediate consistency)
- Снижение нагрузки на БД критично

**Когда НЕ использовать:**
- Real-time updates (нужна immediate consistency)
- Малое количество операций (overhead > benefit)
- Сложная логика (каждая операция уникальна)

**Для нашего проекта:**

Batch processing — опциональное улучшение для Этапа 4. Текущая реализация (single update с worker pool) достаточна для большинства сценариев.

---

## Итого

**Ключевые темы для собеседования:**

1. **Goroutines vs threads** — лёгкость создания, M:N scheduler
2. **Каналы** — буферизированные vs небуферизированные, когда что использовать
3. **Worker pool** — контроль над concurrency, graceful shutdown
4. **sync.WaitGroup** — ожидание завершения группы goroutines
5. **Graceful shutdown** — правильный порядок завершения компонентов
6. **Exponential backoff** — retry стратегия с увеличивающейся задержкой
7. **Batch processing** — группировка операций для снижения нагрузки

**Дополнительные вопросы для самопроверки:**
- Что такое race condition и как его избежать (mutex, atomic, channels)?
- Как работает Go scheduler (GOMAXPROCS, work stealing)?
- Что такое context и зачем он нужен (cancellation, timeout)?
- Как реализовать pub/sub паттерн на каналах?
- Что такое fan-in/fan-out паттерны?

---

## Следующий этап

После изучения этих вопросов переходим к **Этапу 5: Микросервисы** (выделение Stats Service, Kafka).
