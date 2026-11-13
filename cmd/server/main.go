package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/getsentry/sentry-go"
	sentryslog "github.com/getsentry/sentry-go/slog"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/infrastructure/config"
	"github.com/pihpohpah/hermes/internal/infrastructure/openmeteo"
	"github.com/pihpohpah/hermes/internal/infrastructure/repository"
)

func main() {
	ctx := context.Background()

	command := "listen"
	args := []string{}
	const minArgsCount = 2
	if len(os.Args) >= minArgsCount {
		command = os.Args[1]
		args = os.Args[minArgsCount:]
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}

	logger, err := newLogger(ctx, cfg)
	if err != nil {
		log.Fatalf("logger initialization failed: %v", err)
	}

	pool, err := pgxpool.New(ctx, cfg.URL)
	if err != nil {
		log.Fatalf("failed to create pgx pool: %v", err)
	}

	pingCtx, pingCancel := context.WithTimeout(ctx, cfg.PingTimeout)
	if err := pool.Ping(pingCtx); err != nil {
		pingCancel()
		pool.Close()
		log.Fatalf("failed to ping postgres: %v", err)
	}
	pingCancel()

	placeRepo := repository.NewPlaceRepository(pool)
	weatherRepo := repository.NewWeatherRepository(pool)
	forecastRepo := repository.NewForecastRepository(pool)
	tripRepo := repository.NewTripRepository(pool)
	reportRepo := repository.NewReportRepository(pool)
	provider := newWeatherProvider(cfg, logger)
	openMeteoConfig := openmeteo.Config{
		WeatherBaseURL:    cfg.BaseURL,
		MarineBaseURL:     cfg.MarineBaseURL,
		AirQualityBaseURL: cfg.AirQualityBaseURL,
		Timeout:           cfg.Timeout,
		Attempts:          cfg.Attempts,
		BaseBackoff:       cfg.RetryBackoff,
		APIKey:            cfg.APIKey,
	}

	deps := commandDeps{
		logger:          logger,
		places:          placeRepo,
		weather:         weatherRepo,
		forecast:        forecastRepo,
		provider:        provider,
		openMeteoConfig: openMeteoConfig,
	}

	switch command {
	case "listen":
		err = runListen(ctx, logger, cfg, pool, placeRepo, weatherRepo, forecastRepo, tripRepo, reportRepo, provider)
	default:
		err = runCommand(ctx, command, args, deps)
	}

	pool.Close()
	if err != nil {
		log.Fatalf("run %s failed: %v", command, err)
	}
}

func newWeatherProvider(cfg config.Config, logger *slog.Logger) openmeteo.HourlyProvider {
	if cfg.Mode.IsMock() {
		logger.Warn("weather provider runs in MOCK mode: data is generated, not fetched")

		return openmeteo.NewDemoProvider()
	}

	logger.Info(
		"weather provider runs in REAL mode",
		slog.String("weather_base", cfg.BaseURL),
		slog.String("marine_base", cfg.MarineBaseURL),
		slog.String("air_quality_base", cfg.AirQualityBaseURL),
		slog.Duration("timeout", cfg.Timeout),
		slog.Int("attempts", cfg.Attempts),
	)

	return openmeteo.NewClient(openmeteo.Config{
		WeatherBaseURL:    cfg.BaseURL,
		MarineBaseURL:     cfg.MarineBaseURL,
		AirQualityBaseURL: cfg.AirQualityBaseURL,
		Timeout:           cfg.Timeout,
		Attempts:          cfg.Attempts,
		BaseBackoff:       cfg.RetryBackoff,
		APIKey:            cfg.APIKey,
	})
}

func newLogger(ctx context.Context, cfg config.Config) (*slog.Logger, error) {
	opts := &slog.HandlerOptions{
		Level:     cfg.Level,
		AddSource: cfg.AddSource,
	}

	var baseHandler slog.Handler
	if cfg.IsProd() {
		baseHandler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		baseHandler = slog.NewTextHandler(os.Stdout, opts)
	}

	handlers := []slog.Handler{baseHandler}

	if cfg.DSN != "" {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn:              cfg.DSN,
			Environment:      cfg.Environment,
			Release:          cfg.Release,
			Debug:            cfg.Debug,
			EnableTracing:    cfg.TracesSampleRate > 0,
			TracesSampleRate: cfg.TracesSampleRate,
		}); err != nil {
			return nil, fmt.Errorf("initialize sentry: %w", err)
		}

		handlers = append(handlers, sentryslog.Option{
			LogLevel:  []slog.Level{slog.LevelWarn, slog.LevelError, sentryslog.LevelFatal},
			AddSource: cfg.AddSource,
		}.NewSentryHandler(ctx))
	}

	return slog.New(slog.NewMultiHandler(handlers...)), nil
}
