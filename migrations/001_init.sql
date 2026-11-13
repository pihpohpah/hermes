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

CREATE TABLE IF NOT EXISTS places (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    type text NOT NULL CHECK (type IN ('park', 'viewpoint', 'sup_spot', 'trailhead', 'beach')),
    layer_code text NOT NULL DEFAULT 'nature' REFERENCES place_layers(code),
    location geography(Point, 4326) NOT NULL,
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

CREATE TABLE IF NOT EXISTS trips (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL,
    place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
    planned_at timestamptz NOT NULL,
    activity text NOT NULL CHECK (activity IN ('hike', 'sup', 'run', 'walk')),
    status text NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'done', 'cancelled')),
    notes text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

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
    user_id uuid,
    type text NOT NULL CHECK (type IN ('clear', 'fog', 'waves', 'crowded', 'closed')),
    comment text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_places_location ON places USING GIST (location);
CREATE INDEX IF NOT EXISTS idx_places_type ON places USING btree (type);
CREATE INDEX IF NOT EXISTS idx_places_layer_code ON places USING btree (layer_code);
CREATE INDEX IF NOT EXISTS idx_trips_user_planned_at ON trips USING btree (user_id, planned_at);
CREATE INDEX IF NOT EXISTS idx_conditions_cache_forecast_time ON conditions_cache USING btree (forecast_time);
CREATE INDEX IF NOT EXISTS idx_weather_observations_observed_at ON weather_observations USING btree (observed_at);
CREATE INDEX IF NOT EXISTS idx_weather_observations_place_source
    ON weather_observations USING btree (place_id, source, observed_at);
CREATE INDEX IF NOT EXISTS idx_reports_place_created_at ON reports USING btree (place_id, created_at DESC);
