package commands

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

type forecastRefresher interface {
	Do(
		ctx context.Context,
		rng models.ForecastRange,
		activities []models.Activity,
	) (usecase.RefreshReport, error)
}

type ForecastRefreshCommand struct {
	logger     *slog.Logger
	refresher  forecastRefresher
	rng        models.ForecastRange
	activities []models.Activity
}

func NewForecastRefreshCommand(
	logger *slog.Logger,
	refresher forecastRefresher,
	rng models.ForecastRange,
	activities []models.Activity,
) *ForecastRefreshCommand {
	return &ForecastRefreshCommand{
		logger:     logger,
		refresher:  refresher,
		rng:        rng,
		activities: activities,
	}
}

func (c *ForecastRefreshCommand) Run(ctx context.Context) error {
	report, err := c.refresher.Do(ctx, c.rng, c.activities)
	if err != nil {
		return fmt.Errorf("refresh forecast cache: %w", err)
	}

	c.logger.Info(
		"forecast cache refreshed",
		slog.String("day", report.Day),
		slog.Int("horizon_days", report.HorizonDays),
		slog.Int("loaded_hours", report.LoadedHours),
		slog.Int("places", report.Assessed),
		slog.Int("assessments", report.Assessments),
		slog.Int("skipped", len(report.Skipped)),
	)

	return nil
}
