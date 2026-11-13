package config

import (
	"fmt"
	"log/slog"
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
	defaultPMTilesPath       = "./web/static/pmtiles/primorsky-krai-z15.pmtiles"
	defaultReliefPath        = "./web/static/pmtiles/primorsky-relief.pmtiles"
	defaultMapDir            = "./web/map"
	defaultWebDir            = "./web/static"
	defaultReadHeaderTimeout = 10 * time.Second
	defaultShutdownTimeout   = 15 * time.Second
	defaultPingTimeout       = 5 * time.Second
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
	LoggerConfig
	SentryConfig
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
		LoggerConfig: LoggerConfig{
			Level: slog.LevelDebug,
		},
	}
}
