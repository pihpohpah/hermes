package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/application/commands"
	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
	"github.com/pihpohpah/hermes/internal/infrastructure/openmeteo"
	"github.com/pihpohpah/hermes/internal/infrastructure/repository"
)

var ErrUnknownCommand = errors.New("unknown command")

const (
	seedCommand             = "seed"
	elevationsEnrichCommand = "elevations_enrich"
	coverageProbeCommand    = "coverage_probe"
	weatherLoadCommand      = "weather_load"
	conditionsAssessCommand = "conditions_assess"
	forecastRefreshCommand  = "forecast_refresh"
	userCreateCommand       = "user_create"
	pinsCleanupCommand      = "pins_cleanup"
)

type Command interface {
	Run(ctx context.Context) error
}

type commandDeps struct {
	logger          *slog.Logger
	pool            *pgxpool.Pool
	places          *repository.PlaceRepository
	weather         *repository.WeatherRepository
	forecast        *repository.ForecastRepository
	users           *repository.UserRepository
	sessions        *repository.SessionRepository
	idempotency     *repository.IdempotencyRepository
	txManager       *repository.TxManager
	provider        openmeteo.HourlyProvider
	openMeteoConfig openmeteo.Config
}

func runCommand(ctx context.Context, commandName string, args []string, deps commandDeps) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	opts, err := parseCommandOptions(commandName, args)
	if err != nil {
		return err
	}

	rng, err := commands.ResolveForecastRange(opts.day, time.Now())
	if err != nil {
		return fmt.Errorf("resolve forecast range: %w", err)
	}

	var cmd Command
	switch commandName {
	case seedCommand:
		cmd = commands.NewSeedCommand(deps.logger, deps.places)
	case elevationsEnrichCommand:
		enricher := usecase.NewEnrichElevationsUseCase(
			deps.places,
			deps.places,
			openmeteo.NewElevationClient(deps.openMeteoConfig),
		)
		cmd = commands.NewElevationsEnrichCommand(
			deps.logger,
			enricher,
			opts.placeID,
			opts.overwrite,
		)
	case coverageProbeCommand:
		prober := usecase.NewProbeCoverageUseCase(deps.places, deps.provider)
		cmd = commands.NewCoverageProbeCommand(deps.logger, prober, rng, opts.placeID)
	case weatherLoadCommand:
		loader := usecase.NewLoadWeatherUseCase(deps.places, deps.weather, deps.provider)
		cmd = commands.NewWeatherLoadCommand(deps.logger, loader, rng, opts.placeID)
	case conditionsAssessCommand:
		assessor := usecase.NewAssessConditionsUseCase(
			deps.places,
			deps.forecast,
			deps.weather,
			deps.provider,
		)
		cmd = commands.NewConditionsAssessCommand(
			deps.logger,
			assessor,
			rng,
			opts.activities,
			opts.placeID,
		)
	case forecastRefreshCommand:
		refresh := usecase.NewRefreshForecastUseCase(deps.places, deps.forecast, deps.weather, deps.provider)
		cmd = commands.NewForecastRefreshCommand(deps.logger, refresh, rng, opts.activities)
	case userCreateCommand:
		createUser := usecase.NewCreateUserUseCase(
			deps.users,
			deps.sessions,
			deps.idempotency,
			deps.txManager,
			30*24*time.Hour,
		)
		cmd = commands.NewUserCreateCommand(deps.logger, createUser, opts.userName, opts.userTimezone)
	case pinsCleanupCommand:
		pinRepo := repository.NewPinRepository(deps.pool)
		purger := usecase.NewPurgeExpiredPinsUseCase(pinRepo)
		cmd = commands.NewPinsCleanupCommand(deps.logger, purger, opts.pinPurgeGrace)
	default:
		return fmt.Errorf("%w: %s", ErrUnknownCommand, commandName)
	}

	if err := cmd.Run(ctx); err != nil {
		return fmt.Errorf("run command %s: %w", commandName, err)
	}

	return nil
}

type commandOptions struct {
	day           string
	placeID       string
	activities    []models.Activity
	overwrite     bool
	userName      string
	userTimezone  string
	pinPurgeGrace time.Duration
}

func parseCommandOptions(commandName string, args []string) (commandOptions, error) {
	var (
		opts       commandOptions
		actRaw     string
		graceHours float64
	)

	fs := flag.NewFlagSet(commandName, flag.ContinueOnError)
	fs.StringVar(&opts.day, "day", "", "calendar day in Asia/Vladivostok (YYYY-MM-DD), defaults to today")
	fs.StringVar(&opts.placeID, "place", "", "limit the run to one place UUID")
	fs.StringVar(&actRaw, "activity", "", "comma separated activities: walk,sup,hike,run")
	fs.BoolVar(&opts.overwrite, "overwrite", false, "recompute values that already exist (elevations)")
	fs.StringVar(&opts.userName, "name", "", "display name of the new user (user_create)")
	fs.StringVar(&opts.userTimezone, "timezone", "", "IANA timezone of the new user, defaults to Asia/Vladivostok (user_create)")
	fs.Float64Var(&graceHours, "grace_hours", 24, "hours after ends_at before a pin is purged (pins_cleanup)")

	if err := fs.Parse(args); err != nil {
		return commandOptions{}, fmt.Errorf("parse flags for %s: %w", commandName, err)
	}

	activities, err := parseActivities(actRaw)
	if err != nil {
		return commandOptions{}, err
	}
	opts.activities = activities

	if graceHours > 0 {
		opts.pinPurgeGrace = time.Duration(graceHours * float64(time.Hour))
	}

	return opts, nil
}

func parseActivities(raw string) ([]models.Activity, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	activities := make([]models.Activity, 0, len(parts))
	for _, part := range parts {
		activity, err := models.NewActivity(part)
		if err != nil {
			return nil, err
		}
		activities = append(activities, activity)
	}

	return activities, nil
}
