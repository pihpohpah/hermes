package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const (
	defaultPort              = "8080"
	defaultDatabaseURL       = "postgres://postgres:postgres@localhost:5432/hermes?sslmode=disable"
	defaultOpenMeteoBase     = "https://api.open-meteo.com/v1"
	defaultMarineBase        = "https://marine-api.open-meteo.com/v1"
	defaultAirQualityBase    = "https://air-quality-api.open-meteo.com/v1"
	defaultWeatherMode       = "real"
	defaultWeatherTimeout    = 8 * time.Second
	defaultWeatherAttempts   = 3
	defaultWeatherRetryDelay = 300 * time.Millisecond
	defaultEventAgentBase    = "http://127.0.0.1:8000"
	defaultEventAgentTimeout = 30 * time.Second
	defaultEventAgentPoll    = 10 * time.Second
	defaultEventAgentMaxWait = 25 * time.Minute
	defaultGeocoderBase      = "https://nominatim.openstreetmap.org"
	defaultGeocoderTimeout   = 10 * time.Second
	defaultGeocoderUserAgent = "hermes-events-import/1.0"
	defaultGeocoderInterval  = 15 * time.Second
	defaultPMTilesPath       = "./web/static/pmtiles/primorsky-krai-z15.pmtiles"
	defaultReliefPath        = "./web/static/pmtiles/primorsky-relief.pmtiles"
	defaultMapDir            = "./web/map"
	defaultWebDir            = "./web/static"
	defaultReadHeaderTimeout = 10 * time.Second
	defaultShutdownTimeout   = 15 * time.Second
	defaultPingTimeout       = 5 * time.Second
	defaultSessionTTL        = 30 * 24 * time.Hour
	defaultPinsMaxActive     = 5
	defaultPinsMaxPerHour    = 10
	defaultPinsHideThreshold = 3
	defaultPinsMaxDurationH  = 12.0
	defaultPinsPurgeGraceH   = 24.0
	defaultPhotosMaxUploadMB = 10
)

// WeatherMode selects where weather data comes from. A failing real API never
// falls back to demo data: the failure is reported as a failure.
type WeatherMode string

const (
	WeatherModeReal WeatherMode = "real"
	WeatherModeMock WeatherMode = "mock"
)

func (m WeatherMode) IsMock() bool {
	return m == WeatherModeMock
}

type Config struct {
	AppConfig
	DatabaseConfig
	OpenMeteoConfig
	EventAgentConfig
	GeocoderConfig
	LoggerConfig
	SentryConfig
	SessionConfig
	PinsConfig
	PhotosConfig
	TelegramConfig
}

// EventAgentConfig — подключение к HTTP-сервису event-agent (webscraper),
// который выполняет прогон поиска событий. Импорт дёргает POST /runs и опрашивает
// GET /runs/{id}, пока прогон не завершится.
type EventAgentConfig struct {
	BaseURL      string        `koanf:"EVENTAGENT_BASE_URL" validate:"omitempty,url"`
	Timeout      time.Duration `koanf:"EVENTAGENT_TIMEOUT" validate:"required"`
	PollInterval time.Duration `koanf:"EVENTAGENT_POLL_INTERVAL" validate:"required"`
	// MaxWait — потолок ожидания завершения прогона (run_timeout агента + запас).
	MaxWait time.Duration `koanf:"EVENTAGENT_MAX_WAIT" validate:"required"`
}

// GeocoderConfig — геокодинг адресов площадок в координаты. По умолчанию
// публичный Nominatim: без ключа, но не быстрее одного запроса в секунду
// (MinInterval) и с обязательным идентифицирующим User-Agent.
type GeocoderConfig struct {
	BaseURL     string        `koanf:"GEOCODER_BASE_URL" validate:"omitempty,url"`
	Timeout     time.Duration `koanf:"GEOCODER_TIMEOUT" validate:"required"`
	UserAgent   string        `koanf:"GEOCODER_USER_AGENT" validate:"required"`
	MinInterval time.Duration `koanf:"GEOCODER_MIN_INTERVAL" validate:"required"`
}

// TelegramConfig включает вход через Telegram Login library
// (https://core.telegram.org/bots/telegram-login). Пустой ClientID означает
// «вход выключен»: эндпоинт остаётся, но id_token не проверить, а кнопку
// фронтенд не рисует.
type TelegramConfig struct {
	// ClientID — Client ID бота из раздела Login Widget у @BotFather
	// (числовой id бота). Им же проверяется aud id_token.
	ClientID string `koanf:"TELEGRAM_CLIENT_ID"`
	// BotToken — токен бота от @BotFather. Для входа не нужен, остался как
	// основа секрета nonce и для будущих серверных вызовов Bot API.
	BotToken string `koanf:"TELEGRAM_BOT_TOKEN"`
	// APIBase — Bot API; по умолчанию https://api.telegram.org.
	APIBase string `koanf:"TELEGRAM_API_BASE"`
	// ProxyList — список через запятую http(s)-прокси для вызовов к Bot API
	// ("http://user:pass@host:port"). Каждая новая попытка идёт через
	// следующий прокси по кругу; пусто = напрямую.
	ProxyList string `koanf:"TELEGRAM_PROXY_LIST"`
	// MaxAttempts — сколько попыток (смен прокси) делать к Bot API, 0 = 6.
	MaxAttempts int `koanf:"TELEGRAM_MAX_ATTEMPTS"`
}

// ProxyURLList разбирает список прокси, отбрасывая пустые и невалидные —
// один битый элемент не должен ронять весь вход через Telegram.
func (c TelegramConfig) ProxyURLList() []*url.URL {
	parts := strings.Split(c.ProxyList, ",")
	out := make([]*url.URL, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		u, err := url.Parse(part)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		}
		out = append(out, u)
	}

	return out
}

// PhotosConfig covers the photo upload endpoint: the size cap and the local
// storage directory. Files are served by the static handler of the web dir,
// so by default storage lives in <web>/uploads and maps to /uploads URLs.
type PhotosConfig struct {
	// MaxUploadMB caps one multipart upload in megabytes.
	MaxUploadMB int64 `koanf:"PHOTOS_MAX_UPLOAD_MB" validate:"gte=1,lte=50"`
	// StorageDir is the photo storage root. Empty means <WebDir>/uploads.
	StorageDir string `koanf:"PHOTOS_STORAGE_DIR"`
}

// PinsConfig is the moderation policy of the radar «Сейчас»: per-user
// limits, rate limiting, stop words and night hours. Open questions from the
// design TODO are resolved here, per deployment, without code changes.
type PinsConfig struct {
	// MaxActivePerUser caps the number of live pins one author may hold (0 = unlimited).
	MaxActivePerUser int `koanf:"PINS_MAX_ACTIVE_PER_USER"`
	// MaxPerHour caps creations per user in the trailing hour (0 = unlimited).
	MaxPerHour int `koanf:"PINS_MAX_PER_HOUR"`
	// StopWords is a comma-separated list rejected in pin title/note.
	StopWords string `koanf:"PINS_STOP_WORDS"`
	// NightBanFrom/NightBanTo (local HH:MM) reject pin creation in the night
	// window; empty disables the rule.
	NightBanFrom string `koanf:"PINS_NIGHT_BAN_FROM"`
	NightBanTo   string `koanf:"PINS_NIGHT_BAN_TO"`
	// ReportHideThreshold hides a pin after that many distinct complaints (0 = off).
	ReportHideThreshold int `koanf:"PINS_REPORT_HIDE_THRESHOLD"`
	// MaxDurationH caps a pin lifetime in hours.
	MaxDurationH float64 `koanf:"PINS_MAX_DURATION_H"`
	// PurgeGraceH is how long after ends_at a pin is physically deleted.
	PurgeGraceH float64 `koanf:"PINS_PURGE_GRACE_H"`
}

// StopWordList splits the configured stop words, dropping empties.
func (c PinsConfig) StopWordList() []string {
	parts := strings.Split(c.StopWords, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}

	return out
}

type SessionConfig struct {
	// TTL is how long a server-side session stays valid.
	TTL time.Duration `koanf:"SESSION_TTL" validate:"required"`
	// AllowedOrigins is a comma-separated list of origins that may make
	// credentialed cross-origin requests. Empty = same-origin only.
	AllowedOrigins string `koanf:"CORS_ALLOWED_ORIGINS"`
}

// AllowedOriginList splits the configured origins, dropping empties.
func (c SessionConfig) AllowedOriginList() []string {
	parts := strings.Split(c.AllowedOrigins, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}

	return out
}

type AppConfig struct {
	Env               string        `koanf:"APP_ENV" validate:"required"`
	Port              string        `koanf:"PORT" validate:"required"`
	PMTilesPath       string        `koanf:"PMTILES_PATH" validate:"required"`
	ReliefPath        string        `koanf:"RELIEF_PATH" validate:"required"`
	MapDir            string        `koanf:"MAP_DIR" validate:"required"`
	WebDir            string        `koanf:"WEB_DIR" validate:"required"`
	ReadHeaderTimeout time.Duration `koanf:"APP_READ_HEADER_TIMEOUT" validate:"required"`
	ShutdownTimeout   time.Duration `koanf:"APP_SHUTDOWN_TIMEOUT" validate:"required"`
}

func (a AppConfig) Addr() string {
	if strings.HasPrefix(a.Port, ":") {
		return a.Port
	}

	return ":" + a.Port
}

func (a AppConfig) IsProd() bool {
	envName := strings.ToLower(strings.TrimSpace(a.Env))

	return envName == "prod" || envName == "production"
}

type DatabaseConfig struct {
	URL         string        `koanf:"DATABASE_URL" validate:"required"`
	PingTimeout time.Duration `koanf:"DB_PING_TIMEOUT" validate:"required"`
}

type OpenMeteoConfig struct {
	Mode              WeatherMode   `koanf:"WEATHER_MODE" validate:"required,oneof=real mock"`
	BaseURL           string        `koanf:"OPENMETEO_BASE" validate:"required,url"`
	MarineBaseURL     string        `koanf:"OPENMETEO_MARINE_BASE" validate:"required,url"`
	AirQualityBaseURL string        `koanf:"OPENMETEO_AIR_QUALITY_BASE" validate:"required,url"`
	Timeout           time.Duration `koanf:"OPENMETEO_TIMEOUT" validate:"required"`
	Attempts          int           `koanf:"OPENMETEO_ATTEMPTS" validate:"required,gte=1,lte=5"`
	RetryBackoff      time.Duration `koanf:"OPENMETEO_RETRY_BACKOFF" validate:"required"`
	APIKey            string        `koanf:"OPENMETEO_API_KEY"`
}

type LoggerConfig struct {
	Level     slog.Level `koanf:"LOG_LEVEL"`
	AddSource bool       `koanf:"LOG_ADD_SOURCE"`
}

type SentryConfig struct {
	DSN              string  `koanf:"SENTRY_DSN" validate:"omitempty,url"`
	Environment      string  `koanf:"SENTRY_ENVIRONMENT"`
	Release          string  `koanf:"SENTRY_RELEASE"`
	Debug            bool    `koanf:"SENTRY_DEBUG"`
	TracesSampleRate float64 `koanf:"SENTRY_TRACES_SAMPLE_RATE" validate:"gte=0,lte=1"`
}

func Load() (Config, error) {
	k := koanf.New("")

	if _, err := os.Stat(".env"); err == nil {
		if err := k.Load(file.Provider(".env"), dotenv.ParserEnv(".", "=", nil)); err != nil {
			return Config{}, fmt.Errorf("load config file `.env`: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return Config{}, fmt.Errorf("stat config file `.env`: %w", err)
	}

	if err := k.Load(env.Provider("", ".", nil), nil); err != nil {
		return Config{}, fmt.Errorf("load config from env: %w", err)
	}

	cfg := defaultCfg()
	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{
		DecoderConfig: &mapstructure.DecoderConfig{
			Squash:           true,
			Result:           &cfg,
			WeaklyTypedInput: true,
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				mapstructure.TextUnmarshallerHookFunc(),
			),
		},
	}); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := validator.New().Struct(cfg); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

func defaultCfg() Config {
	return Config{
		AppConfig: AppConfig{
			Env:               "dev",
			Port:              defaultPort,
			PMTilesPath:       defaultPMTilesPath,
			ReliefPath:        defaultReliefPath,
			MapDir:            defaultMapDir,
			WebDir:            defaultWebDir,
			ReadHeaderTimeout: defaultReadHeaderTimeout,
			ShutdownTimeout:   defaultShutdownTimeout,
		},
		DatabaseConfig: DatabaseConfig{
			URL:         defaultDatabaseURL,
			PingTimeout: defaultPingTimeout,
		},
		OpenMeteoConfig: OpenMeteoConfig{
			Mode:              defaultWeatherMode,
			BaseURL:           defaultOpenMeteoBase,
			MarineBaseURL:     defaultMarineBase,
			AirQualityBaseURL: defaultAirQualityBase,
			Timeout:           defaultWeatherTimeout,
			Attempts:          defaultWeatherAttempts,
			RetryBackoff:      defaultWeatherRetryDelay,
		},
		EventAgentConfig: EventAgentConfig{
			BaseURL:      defaultEventAgentBase,
			Timeout:      defaultEventAgentTimeout,
			PollInterval: defaultEventAgentPoll,
			MaxWait:      defaultEventAgentMaxWait,
		},
		GeocoderConfig: GeocoderConfig{
			BaseURL:     defaultGeocoderBase,
			Timeout:     defaultGeocoderTimeout,
			UserAgent:   defaultGeocoderUserAgent,
			MinInterval: defaultGeocoderInterval,
		},
		LoggerConfig: LoggerConfig{
			Level: slog.LevelDebug,
		},
		SessionConfig: SessionConfig{
			TTL: defaultSessionTTL,
		},
		PinsConfig: PinsConfig{
			MaxActivePerUser:    defaultPinsMaxActive,
			MaxPerHour:          defaultPinsMaxPerHour,
			ReportHideThreshold: defaultPinsHideThreshold,
			MaxDurationH:        defaultPinsMaxDurationH,
			PurgeGraceH:         defaultPinsPurgeGraceH,
		},
		PhotosConfig: PhotosConfig{
			MaxUploadMB: defaultPhotosMaxUploadMB,
		},
	}
}
