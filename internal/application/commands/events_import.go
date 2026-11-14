package commands

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pihpohpah/hermes/internal/application/usecase"
)

type eventsImporter interface {
	Do(ctx context.Context) (usecase.ImportReport, error)
}

// EventsImportCommand запускает прогон event-agent и импортирует найденные
// события в каталог (с геокодингом площадок), идемпотентно по external_id.
type EventsImportCommand struct {
	logger   *slog.Logger
	importer eventsImporter
}

func NewEventsImportCommand(logger *slog.Logger, importer eventsImporter) *EventsImportCommand {
	return &EventsImportCommand{logger: logger, importer: importer}
}

func (c *EventsImportCommand) Run(ctx context.Context) error {
	report, err := c.importer.Do(ctx)
	if err != nil {
		return fmt.Errorf("import events: %w", err)
	}

	c.logger.Info(
		"events import finished",
		slog.String("run_id", report.RunID),
		slog.String("run_status", string(report.Status)),
		slog.Int("created", report.Created),
		slog.Int("updated", report.Updated),
		slog.Int("venues_geocoded", report.Geocoded),
		slog.Int("venues_geocode_failed", report.GeocodeFailed),
		slog.Int("events_without_venue", report.NoVenue),
		slog.Duration("took", report.FinishedAt.Sub(report.StartedAt)),
	)

	return nil
}
