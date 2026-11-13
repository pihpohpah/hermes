package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// elevationProvider resolves ground elevation for many points at once.
type elevationProvider interface {
	FetchElevations(ctx context.Context, points []models.Point) ([]models.Measure, error)
}

type elevationWriter interface {
	SetElevation(
		ctx context.Context,
		placeID string,
		meters int,
		source models.ElevationSource,
	) error
}

// EnrichElevationsUseCase fills missing place altitudes. Without an altitude a
// mountainous point is scored with a city forecast, which is exactly the
// mistake this use case removes.
type EnrichElevationsUseCase struct {
	places   placeReader
	writer   elevationWriter
	provider elevationProvider
}

func NewEnrichElevationsUseCase(
	places placeReader,
	writer elevationWriter,
	provider elevationProvider,
) *EnrichElevationsUseCase {
	return &EnrichElevationsUseCase{places: places, writer: writer, provider: provider}
}

type ElevationReport struct {
	Updated    int
	Skipped    int
	Failed     []PlaceFailure
	StartedAt  time.Time
	FinishedAt time.Time
}

// Do resolves elevations for places lacking one. Existing manual values are
// never overwritten unless overwrite is set.
func (u *EnrichElevationsUseCase) Do(
	ctx context.Context,
	placeID string,
	overwrite bool,
) (ElevationReport, error) {
	report := ElevationReport{StartedAt: time.Now().UTC()}

	places, err := selectPlaces(ctx, u.places, placeID)
	if err != nil {
		return report, err
	}

	targets := make([]models.Place, 0, len(places))
	for _, place := range places {
		if place.Elevation != nil && !overwrite {
			report.Skipped++

			continue
		}
		targets = append(targets, place)
	}
	if len(targets) == 0 {
		report.FinishedAt = time.Now().UTC()

		return report, nil
	}

	points := make([]models.Point, len(targets))
	for i, place := range targets {
		points[i] = place.Location
	}

	elevations, err := u.provider.FetchElevations(ctx, points)
	if err != nil {
		return report, fmt.Errorf("resolve elevations: %w", err)
	}
	if len(elevations) != len(targets) {
		return report, fmt.Errorf(
			"resolve elevations: expected %d values, got %d", len(targets), len(elevations),
		)
	}

	for i, place := range targets {
		value := elevations[i]
		if !value.Known {
			report.Failed = append(report.Failed, PlaceFailure{
				PlaceID:   place.ID,
				PlaceName: place.Name,
				Reason:    "источник не вернул высоту",
			})

			continue
		}

		if err := u.writer.SetElevation(
			ctx,
			place.ID,
			int(value.Value),
			models.ElevationSourceOpenMeteo,
		); err != nil {
			return report, fmt.Errorf("save elevation for place `%s`: %w", place.ID, err)
		}
		report.Updated++
	}

	report.FinishedAt = time.Now().UTC()

	return report, nil
}
