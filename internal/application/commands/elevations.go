package commands

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pihpohpah/hermes/internal/application/usecase"
)

type elevationEnricher interface {
	Do(ctx context.Context, placeID string, overwrite bool) (usecase.ElevationReport, error)
}

// ElevationsEnrichCommand resolves ground altitude for places that lack it, so
// mountain and city points stop sharing one coarse model cell.
type ElevationsEnrichCommand struct {
	logger    *slog.Logger
	enricher  elevationEnricher
	placeID   string
	overwrite bool
}

func NewElevationsEnrichCommand(
	logger *slog.Logger,
	enricher elevationEnricher,
	placeID string,
	overwrite bool,
) *ElevationsEnrichCommand {
	return &ElevationsEnrichCommand{
		logger:    logger,
		enricher:  enricher,
		placeID:   placeID,
		overwrite: overwrite,
	}
}

func (c *ElevationsEnrichCommand) Run(ctx context.Context) error {
	report, err := c.enricher.Do(ctx, c.placeID, c.overwrite)
	if err != nil {
		return fmt.Errorf("enrich elevations: %w", err)
	}

	c.logger.Info(
		"elevations resolved",
		slog.Int("updated", report.Updated),
		slog.Int("skipped", report.Skipped),
		slog.Int("failed", len(report.Failed)),
		slog.Duration("took", report.FinishedAt.Sub(report.StartedAt)),
	)
	for _, failure := range report.Failed {
		c.logger.Warn(
			"elevation not resolved",
			slog.String("place_id", failure.PlaceID),
			slog.String("place", failure.PlaceName),
			slog.String("reason", failure.Reason),
		)
	}

	return nil
}
