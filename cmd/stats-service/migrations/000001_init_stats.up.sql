-- Таблица для хранения событий кликов (сырые данные)
CREATE TABLE IF NOT EXISTS click_events (
    id BIGSERIAL PRIMARY KEY,
    link_id BIGINT NOT NULL,
    short_code VARCHAR(16) NOT NULL,
    user_agent TEXT,
    referer TEXT,
    clicked_at TIMESTAMPTZ DEFAULT NOW()
);

-- Индекс для быстрого поиска кликов по link_id
CREATE INDEX IF NOT EXISTS idx_click_events_link_id ON click_events(link_id);

-- Индекс для аналитики по времени
CREATE INDEX IF NOT EXISTS idx_click_events_clicked_at ON click_events(clicked_at);

-- Таблица агрегированной статистики (для быстрых запросов)
CREATE TABLE IF NOT EXISTS link_stats (
    link_id BIGINT PRIMARY KEY,
    short_code VARCHAR(16) NOT NULL,
    original_url TEXT NOT NULL,
    total_clicks BIGINT DEFAULT 0,
    unique_clicks BIGINT DEFAULT 0,
    last_clicked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Индекс для топ популярных ссылок
CREATE INDEX IF NOT EXISTS idx_link_stats_total_clicks ON link_stats(total_clicks DESC);

-- Индекс для "горячих" ссылок (trending)
CREATE INDEX IF NOT EXISTS idx_link_stats_last_clicked_at ON link_stats(last_clicked_at DESC);
