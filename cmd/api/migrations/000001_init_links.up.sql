CREATE TABLE IF NOT EXISTS links (
    id BIGSERIAL PRIMARY KEY,
    short_code VARCHAR(16) UNIQUE NOT NULL,
    original_url TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    clicks BIGINT DEFAULT 0,
    last_accessed_at TIMESTAMPTZ
);

-- Индекс для быстрого поиска по short_code (уже UNIQUE, но явно для ясности)
CREATE INDEX IF NOT EXISTS idx_links_short_code ON links(short_code);

-- Индекс для поиска дубликатов original_url (при создании ссылки)
CREATE INDEX IF NOT EXISTS idx_links_original_url ON links(original_url);
