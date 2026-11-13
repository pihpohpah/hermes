package commands

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

type weatherLoader interface {
	Do(ctx context.Context, rng models.ForecastRange, placeID string) (usecase.WeatherLoadReport, error)
}

// WeatherLoadCommand downloads hourly weather for all places (or a single one)
// and stores it as raw observations.
type WeatherLoadCommand struct {
	logger  *slog.Logger
	loader  weatherLoader
	rng     models.ForecastRange
	placeID string
}

func NewWeatherLoadCommand(
	logger *slog.Logger,
	loader weatherLoader,
	rng models.ForecastRange,
	placeID string,
) *WeatherLoadCommand {
	return &WeatherLoadCommand{logger: logger, loader: loader, rng: rng, placeID: placeID}
}

func (c *WeatherLoadCommand) Run(ctx context.Context) error {
	report, err := c.loader.Do(ctx, c.rng, c.placeID)
	if err != nil {
		return fmt.Errorf("load weather from %s: %w", c.rng.From.Format(time.RFC3339), err)
	}

	c.logger.Info(
		"weather loaded",
		slog.String("day", report.Day),
		slog.Int("horizon_days", report.HorizonDays),
		slog.String("from", report.From.Format(time.RFC3339)),
		slog.String("to", report.To.Format(time.RFC3339)),
		slog.Int("places", report.LoadedPlaces),
		slog.Int("hours", report.LoadedHours),
		slog.Int("failed", len(report.Failed)),
		slog.Duration("took", report.FinishedAt.Sub(report.StartedAt)),
	)
	for _, item := range report.Coverage {
		c.logger.Info(
			"source coverage",
			slog.String("source", string(item.Kind)),
			slog.Bool("available", item.Available),
			slog.Int("hours", item.Covered),
			slog.String("to", item.To),
			slog.String("reason", item.Reason),
		)
	}
	for _, failure := range report.Failed {
		c.logger.Warn(
			"place weather failed",
			slog.String("place_id", failure.PlaceID),
			slog.String("place", failure.PlaceName),
			slog.String("reason", failure.Reason),
		)
	}

	return nil
}

// ConditionsAssessCommand runs the domain scorer over stored observations.
type ConditionsAssessCommand struct {
	logger     *slog.Logger
	assessor   conditionsAssessor
	rng        models.ForecastRange
	activities []models.Activity
	placeID    string
}

type conditionsAssessor interface {
	Do(
		ctx context.Context,
		rng models.ForecastRange,
		activities []models.Activity,
		placeID string,
	) (usecase.AssessReport, error)
}

func NewConditionsAssessCommand(
	logger *slog.Logger,
	assessor conditionsAssessor,
	rng models.ForecastRange,
	activities []models.Activity,
	placeID string,
) *ConditionsAssessCommand {
	return &ConditionsAssessCommand{
		logger:     logger,
		assessor:   assessor,
		rng:        rng,
		activities: activities,
		placeID:    placeID,
	}
}

func (c *ConditionsAssessCommand) Run(ctx context.Context) error {
	report, err := c.assessor.Do(ctx, c.rng, c.activities, c.placeID)
	if err != nil {
		return fmt.Errorf("assess conditions from %s: %w", c.rng.From.Format(time.RFC3339), err)
	}

	activities := make([]string, 0, len(report.Activities))
	for _, activity := range report.Activities {
		activities = append(activities, string(activity))
	}
	c.logger.Info(
		"conditions assessed",
		slog.String("day", report.Day),
		slog.Int("places", report.Assessed),
		slog.Int("assessments", report.Assessments),
		slog.Int("hours", report.Hours),
		slog.Any("activities", activities),
		slog.Int("skipped", len(report.Skipped)),
	)
	for _, skipped := range report.Skipped {
		c.logger.Warn(
			"place conditions skipped",
			slog.String("place_id", skipped.PlaceID),
			slog.String("place", skipped.PlaceName),
			slog.String("reason", skipped.Reason),
		)
	}

	return nil
}

// ResolveForecastRange builds the rolling 10-day range. The raw argument only
// shifts the starting date for diagnostics; the default is "now".
func ResolveForecastRange(raw string, now time.Time) (models.ForecastRange, error) {
	if raw == "" {
		return models.NewForecastRange(now)
	}

	window, err := models.ParseDayWindow(raw)
	if err != nil {
		return models.ForecastRange{}, err
	}

	return models.NewForecastRange(window.Start)
}
