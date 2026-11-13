CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE IF NOT EXISTS place_layers (
    code text PRIMARY KEY CHECK (code IN ('city', 'nature', 'sea')),
    title text NOT NULL
);

INSERT INTO place_layers (code, title) VALUES
    ('city', 'Город'),
    ('nature', 'Природа'),
    ('sea', 'Море')
ON CONFLICT (code) DO NOTHING;

-- Категории мест по сценариям (как в taxonomy.js прототипа).
CREATE TABLE IF NOT EXISTS place_categories (
    code text PRIMARY KEY,
    scenario text NOT NULL CHECK (scenario IN ('city', 'nature', 'sea')),
    title text NOT NULL
);

INSERT INTO place_categories (code, scenario, title) VALUES
    ('walks', 'city', 'Прогулки'),
    ('viewpoints', 'city', 'Смотровые'),
    ('history', 'city', 'История'),
    ('museums', 'city', 'Музеи'),
    ('art', 'city', 'Арт-пространства'),
    ('stage', 'city', 'Музыка и сцена'),
    ('hiking', 'nature', 'Сопки и тропы'),
    ('waterfalls', 'nature', 'Водопады'),
    ('lakes', 'nature', 'Озёра'),
    ('parks', 'nature', 'Парки и сады'),
    ('capes', 'nature', 'Мысы и скалы'),
    ('reserves', 'nature', 'Заповедные места'),
    ('beaches', 'sea', 'Пляжи'),
    ('bays', 'sea', 'Бухты'),
    ('lighthouses', 'sea', 'Маяки'),
    ('islands', 'sea', 'Острова'),
    ('diving', 'sea', 'Дайвинг'),
    ('boats', 'sea', 'Морские прогулки')
ON CONFLICT (code) DO NOTHING;

-- Категории событий.
CREATE TABLE IF NOT EXISTS event_categories (
    code text PRIMARY KEY,
    title text NOT NULL
);

INSERT INTO event_categories (code, title) VALUES
    ('concert', 'Концерт'),
    ('exhibition', 'Выставка'),
    ('tour', 'Экскурсия'),
    ('festival', 'Фестиваль'),
    ('sport', 'Спорт'),
    ('party', 'Андеграунд'),
    ('lecture', 'Лекция'),
    ('market', 'Маркет')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS places (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text UNIQUE,
    name text NOT NULL,
    subtitle text NOT NULL DEFAULT '',
    short text NOT NULL DEFAULT '',
    description text NOT NULL DEFAULT '',
    type text NOT NULL CHECK (type IN ('park', 'viewpoint', 'sup_spot', 'trailhead', 'beach')),
    layer_code text NOT NULL DEFAULT 'nature' REFERENCES place_layers(code),
    category text REFERENCES place_categories(code),
    district text NOT NULL DEFAULT '',
    address text NOT NULL DEFAULT '',
    location geography(Point, 4326) NOT NULL,
    hours text NOT NULL DEFAULT '',
    price_free boolean,
    price_label text NOT NULL DEFAULT '',
    duration text NOT NULL DEFAULT '',
    season text NOT NULL DEFAULT '',
    access text[] NOT NULL DEFAULT '{}' CHECK (
        access <@ ARRAY['walk', 'transit', 'car', 'ferry', 'boat']::text[]
    ),
    how_to_get text NOT NULL DEFAULT '',
    facts text[] NOT NULL DEFAULT '{}',
    tags text[] NOT NULL DEFAULT '{}',
    photos jsonb NOT NULL DEFAULT '[]'::jsonb,
    beach text CHECK (beach IS NULL OR beach IN ('sand', 'pebble', 'rocks', 'none')),
    swim_season text NOT NULL DEFAULT '',
    elevation int,
    elevation_source text CHECK (elevation_source IN ('manual', 'open_meteo', 'dem')),
    difficulty int CHECK (difficulty BETWEEN 1 AND 5),
    osm_id bigint UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS place_sources (
    place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
    source text NOT NULL CHECK (source IN ('osm', 'editorial', 'community')),
    external_id text NOT NULL,
    synced_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (place_id, source)
);

-- Событие: то же, что место, плюс время, площадка и организатор.
-- Место может быть не привязано (venue хранит свободный текст).
CREATE TABLE IF NOT EXISTS events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text UNIQUE,
    title text NOT NULL,
    category text NOT NULL REFERENCES event_categories(code),
    short text NOT NULL DEFAULT '',
    description text NOT NULL DEFAULT '',
    place_id uuid REFERENCES places(id) ON DELETE SET NULL,
    venue text NOT NULL DEFAULT '',
    organizer text NOT NULL DEFAULT '',
    age text NOT NULL DEFAULT '',
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT events_time_window CHECK (ends_at > starts_at)
);

-- Гостевой профиль: постоянный идентификатор пользователя, к которому
-- привязываются поездки и репорты. Пароля, email и ролей здесь нет.
--
-- Расширяемость под сторонние сервисы авторизации: новые способы входа
-- добавляются строками в user_identities, а не колонками в users. Поэтому
CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name text NOT NULL,
    timezone text NOT NULL DEFAULT 'Asia/Vladivostok',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_display_name_length CHECK (char_length(display_name) BETWEEN 1 AND 100)
);

-- Внешние идентичности (Telegram, OAuth и т.п.). Провайдер и субъект
-- уникальны в паре: один пользователь может иметь несколько способов входа.
CREATE TABLE IF NOT EXISTS user_identities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider text NOT NULL,
    subject text NOT NULL,
    data jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_identities_provider_subject_key UNIQUE (provider, subject)
);

CREATE INDEX IF NOT EXISTS idx_user_identities_user_id ON user_identities USING btree (user_id);

-- Серверная сессия: в БД лежит только хеш непрозрачного токена, поэтому утечка
-- таблицы не даёт готовых cookie. CSRF-секрет тоже хранится хешем.
CREATE TABLE IF NOT EXISTS user_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash text NOT NULL,
    csrf_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_used_at timestamptz,
    revoked_at timestamptz,
    CONSTRAINT user_sessions_token_hash_key UNIQUE (token_hash)
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions USING btree (user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions USING btree (expires_at);

-- Идемпотентность создания пользователя. Ключ размечается scope, чтобы тем же
-- механизмом позже воспользоваться другим изменяющим эндпоинтам.
CREATE TABLE IF NOT EXISTS idempotency_keys (
    scope text NOT NULL,
    key text NOT NULL,
    request_fingerprint text NOT NULL,
    user_id uuid REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (scope, key)
);

CREATE INDEX IF NOT EXISTS idx_idempotency_keys_expires_at ON idempotency_keys USING btree (expires_at);

-- Демо-владелец для записей, созданных до появления пользователей. Фиксированный
-- UUID совпадает с тем, который фронтенд использовал как «гостя».
INSERT INTO users (id, display_name, timezone)
VALUES ('00000000-0000-0000-0000-000000000001', 'Демо-гость', 'Asia/Vladivostok')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS trips (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
    planned_at timestamptz NOT NULL,
    activity text NOT NULL CHECK (activity IN ('hike', 'sup', 'run', 'walk')),
    status text NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'done', 'cancelled')),
    notes text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Виды меток «Сейчас» (sport/walk/music/games/help/other) — отдельный
-- справочник, не связанный с activity (hike/sup/run/walk) скоринга условий.
CREATE TABLE IF NOT EXISTS pin_kinds (
    code text PRIMARY KEY CHECK (code IN ('sport', 'walk', 'music', 'games', 'help', 'other')),
    title text NOT NULL
);

INSERT INTO pin_kinds (code, title) VALUES
    ('sport', 'Спорт'),
    ('walk', 'Прогулка'),
    ('music', 'Музыка'),
    ('games', 'Игры'),
    ('help', 'Помощь'),
    ('other', 'Другое')
ON CONFLICT (code) DO NOTHING;

-- Метка живёт от starts_at до ends_at и затем исчезает с карты. Жалобы
-- прячут метку (hidden), физическое удаление — фоновой задачей после grace-периода.
CREATE TABLE IF NOT EXISTS pins (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind text NOT NULL REFERENCES pin_kinds(code),
    title text NOT NULL,
    note text NOT NULL DEFAULT '',
    place_label text NOT NULL DEFAULT '',
    location geography(Point, 4326) NOT NULL,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    need int CHECK (need IS NULL OR need BETWEEN 0 AND 100),
    hidden boolean NOT NULL DEFAULT false,
    hidden_reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pins_time_window CHECK (ends_at > starts_at),
    CONSTRAINT pins_title_length CHECK (char_length(title) BETWEEN 1 AND 140),
    CONSTRAINT pins_note_length CHECK (char_length(note) <= 140),
    CONSTRAINT pins_place_label_length CHECK (char_length(place_label) <= 140)
);

-- «Я приду»: одна запись на пару (метка, пользователь). Автор идёт по
-- определению, поэтому счётчик going = count(pin_goings) + 1.
CREATE TABLE IF NOT EXISTS pin_goings (
    pin_id uuid NOT NULL REFERENCES pins(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (pin_id, user_id)
);

CREATE TABLE IF NOT EXISTS pin_reports (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pin_id uuid NOT NULL REFERENCES pins(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pin_reports_reason_length CHECK (char_length(reason) BETWEEN 1 AND 280)
);

-- Один пользователь — одна жалоба на метку; уникальность поддерживает
-- пересчёт порога скрытия без гонок повторных жалоб.
CREATE UNIQUE INDEX IF NOT EXISTS idx_pin_reports_pin_user ON pin_reports (pin_id, user_id);
CREATE INDEX IF NOT EXISTS idx_pin_reports_pin_created_at ON pin_reports USING btree (pin_id, created_at DESC);

-- Почасовая погода по источникам. Одна строка = один источник × один час,
-- поэтому море, погода и качество воздуха обновляются и живут независимо.
-- Значения, которых нет в ответе API, хранятся как NULL и НЕ подменяются нулём.
CREATE TABLE IF NOT EXISTS weather_observations (
    place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
    observed_at timestamptz NOT NULL,
    source text NOT NULL CHECK (source IN (
        'open_meteo_weather', 'open_meteo_marine', 'open_meteo_air_quality'
    )),
    air_temp_c double precision,
    apparent_temp_c double precision,
    precipitation_mm double precision,
    wind_speed_ms double precision,
    wind_gust_ms double precision,
    wind_direction_deg double precision,
    relative_humidity_pct double precision,
    cloud_cover_pct double precision,
    visibility_m double precision,
    weather_code double precision,
    wave_height_m double precision,
    wave_period_s double precision,
    wave_direction_deg double precision,
    water_temp_c double precision,
    sea_level_height_m double precision,
    air_quality_index double precision,
    pm2_5 double precision,
    model_geometry jsonb NOT NULL DEFAULT '{}'::jsonb,
    valid_until timestamptz,
    fetched_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (place_id, source, observed_at)
);

-- Статус и покрытие по каждому источнику погоды для точки.
CREATE TABLE IF NOT EXISTS weather_source_status (
    place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
    source text NOT NULL CHECK (source IN (
        'open_meteo_weather', 'open_meteo_marine', 'open_meteo_air_quality'
    )),
    available boolean NOT NULL,
    reason text NOT NULL DEFAULT '',
    demo boolean NOT NULL DEFAULT false,
    covered_from timestamptz,
    covered_to timestamptz,
    covered_hours int NOT NULL DEFAULT 0,
    fetched_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (place_id, source)
);

-- Оценки условий. Комфорт (score) и безопасность (verdict, warnings)
-- разделены, версия алгоритма фиксируется для каждой строки.
CREATE TABLE IF NOT EXISTS conditions_cache (
    place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
    forecast_time timestamptz NOT NULL,
    activity text NOT NULL DEFAULT 'walk' CHECK (activity IN ('hike', 'sup', 'run', 'walk')),
    score int NOT NULL CHECK (score BETWEEN 0 AND 100),
    verdict text NOT NULL DEFAULT 'caution' CHECK (verdict IN ('go', 'caution', 'no_go')),
    confidence text NOT NULL DEFAULT 'low' CHECK (confidence IN ('high', 'medium', 'low')),
    data jsonb NOT NULL DEFAULT '{}'::jsonb,
    factors jsonb NOT NULL DEFAULT '{}'::jsonb,
    contributions jsonb NOT NULL DEFAULT '[]'::jsonb,
    warnings jsonb NOT NULL DEFAULT '[]'::jsonb,
    limitations jsonb NOT NULL DEFAULT '[]'::jsonb,
    summary text NOT NULL DEFAULT '',
    sources jsonb NOT NULL DEFAULT '[]'::jsonb,
    demo boolean NOT NULL DEFAULT false,
    algorithm_version text NOT NULL DEFAULT 'unknown',
    computed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (place_id, activity, forecast_time)
);

CREATE TABLE IF NOT EXISTS reports (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
    user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    type text NOT NULL CHECK (type IN ('clear', 'fog', 'waves', 'crowded', 'closed')),
    comment text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Редакционные подборки справочника: сохранённые списки, а не предикаты.
CREATE TABLE IF NOT EXISTS collections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text UNIQUE,
    title text NOT NULL,
    scenario text CHECK (scenario IS NULL OR scenario IN ('city', 'nature', 'sea')),
    cover text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS collection_items (
    collection_id uuid NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    item_type text NOT NULL CHECK (item_type IN ('place', 'event')),
    item_id uuid NOT NULL,
    position int NOT NULL DEFAULT 0,
    PRIMARY KEY (collection_id, item_type, item_id)
);

CREATE INDEX IF NOT EXISTS idx_places_location ON places USING GIST (location);
CREATE INDEX IF NOT EXISTS idx_places_type ON places USING btree (type);
CREATE INDEX IF NOT EXISTS idx_places_layer_code ON places USING btree (layer_code);
CREATE INDEX IF NOT EXISTS idx_places_category ON places USING btree (category);
CREATE INDEX IF NOT EXISTS idx_events_starts_at ON events USING btree (starts_at);
CREATE INDEX IF NOT EXISTS idx_events_place_id ON events USING btree (place_id);
CREATE INDEX IF NOT EXISTS idx_events_category ON events USING btree (category);
CREATE INDEX IF NOT EXISTS idx_pins_location ON pins USING GIST (location);
CREATE INDEX IF NOT EXISTS idx_pins_ends_at ON pins USING btree (ends_at);
CREATE INDEX IF NOT EXISTS idx_pins_author_ends_at ON pins USING btree (author_id, ends_at);
CREATE INDEX IF NOT EXISTS idx_pin_goings_user_id ON pin_goings USING btree (user_id);
CREATE INDEX IF NOT EXISTS idx_trips_user_planned_at ON trips USING btree (user_id, planned_at);
CREATE INDEX IF NOT EXISTS idx_trips_user_id ON trips USING btree (user_id);
CREATE INDEX IF NOT EXISTS idx_conditions_cache_forecast_time ON conditions_cache USING btree (forecast_time);
CREATE INDEX IF NOT EXISTS idx_weather_observations_observed_at ON weather_observations USING btree (observed_at);
CREATE INDEX IF NOT EXISTS idx_weather_observations_place_source
    ON weather_observations USING btree (place_id, source, observed_at);
CREATE INDEX IF NOT EXISTS idx_reports_place_created_at ON reports USING btree (place_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_reports_user_id ON reports USING btree (user_id);
CREATE INDEX IF NOT EXISTS idx_collection_items_item ON collection_items USING btree (item_type, item_id);
CREATE INDEX IF NOT EXISTS idx_collections_scenario ON collections USING btree (scenario);
