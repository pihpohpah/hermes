package commands

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

type coverageProber interface {
	Do(ctx context.Context, rng models.ForecastRange, placeID string) (usecase.CoverageReport, error)
}

// CoverageProbeCommand prints what each source actually returned for every
// place, so gaps are validated across all map points instead of a few seeds.
type CoverageProbeCommand struct {
	logger  *slog.Logger
	prober  coverageProber
	rng     models.ForecastRange
	placeID string
}

func NewCoverageProbeCommand(
	logger *slog.Logger,
	prober coverageProber,
	rng models.ForecastRange,
	placeID string,
) *CoverageProbeCommand {
	return &CoverageProbeCommand{logger: logger, prober: prober, rng: rng, placeID: placeID}
}

func (c *CoverageProbeCommand) Run(ctx context.Context) error {
	report, err := c.prober.Do(ctx, c.rng, c.placeID)
	if err != nil {
		return fmt.Errorf("probe coverage from %s: %w", c.rng.From.Format(time.RFC3339), err)
	}

	c.logger.Info(
		"coverage probed",
		slog.String("day", report.Day),
		slog.Int("horizon_days", report.HorizonDays),
		slog.Int("places", len(report.Probes)),
		slog.Int("model_cells", report.CellCount()),
		slog.Int("shared_cells", len(report.SharedCells)),
		slog.Int("failed", len(report.Missing)),
	)

	sort.Slice(report.Probes, func(i, j int) bool {
		return report.Probes[i].PlaceName < report.Probes[j].PlaceName
	})
	for _, probe := range report.Probes {
		if probe.Error != "" {
			c.logger.Warn(
				"place has no data",
				slog.String("place", probe.PlaceName),
				slog.String("type", string(probe.PlaceType)),
				slog.String("reason", probe.Error),
			)

			continue
		}
		c.logger.Info(
			"place data",
			slog.String("place", probe.PlaceName),
			slog.String("type", string(probe.PlaceType)),
			slog.String("layer", string(probe.Layer)),
			slog.String("elevation", elevationText(probe)),
			slog.String("terrain", probe.Geometry.Terrain),
			slog.String("cell", probe.CellKey()),
			slog.String("sources", sourcesText(probe.Sources)),
			slog.String("fields", fieldsText(probe)),
			slog.Int("hours", probe.Hours),
		)
	}

	for _, group := range report.SharedCells {
		c.logger.Warn(
			"places share one model cell",
			slog.String("cell", group.Cell),
			slog.String("places", strings.Join(group.Places, ", ")),
			slog.String("hint", "погода для них придёт одинаковой; высота точки корректирует только температуру"),
		)
	}

	return nil
}

func elevationText(probe usecase.CoverageProbe) string {
	if probe.Elevation == nil {
		return "нет данных"
	}

	source := probe.Geometry.ElevationSource
	if source == "" {
		source = "не указан"
	}

	return fmt.Sprintf("%d м (%s)", *probe.Elevation, source)
}

func sourcesText(sources []models.SourceStatus) string {
	parts := make([]string, 0, len(sources))
	for _, source := range sources {
		state := "ok"
		if !source.Available {
			state = "нет"
		}
		if source.Demo {
			state = "demo"
		}
		parts = append(parts, fmt.Sprintf("%s=%s", source.Source, state))
	}

	return strings.Join(parts, " ")
}

func fieldsText(probe usecase.CoverageProbe) string {
	parts := make([]string, 0, len(probe.FieldCounts))
	for factor, count := range probe.FieldCounts {
		if probe.Hours == 0 {
			continue
		}
		if count == probe.Hours {
			parts = append(parts, string(factor))
		} else if count > 0 {
			parts = append(parts, fmt.Sprintf("%s(%d/%d)", factor, count, probe.Hours))
		}
	}
	sort.Strings(parts)

	return strings.Join(parts, ",")
}
