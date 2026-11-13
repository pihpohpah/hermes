package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// CoverageProbe is what one source actually returned for one place.
type CoverageProbe struct {
	PlaceID   string
	PlaceName string
	PlaceType models.PlaceType
	Layer     models.LayerCode
	Elevation *int

	Sources     []models.SourceStatus
	Slices      []models.SourceSlice
	Geometry    models.ModelGeometry
	FieldCounts map[models.Factor]int
	Hours       int
	Error       string
}

// SourceCoverage converts the probe into per-source coverage rows.
func (p CoverageProbe) SourceCoverage() []models.SourceCoverage {
	coverage := make([]models.SourceCoverage, 0, len(p.Slices))
	for _, slice := range p.Slices {
		item := models.SourceCoverage{
			Kind:      slice.Kind,
			Available: slice.Status.Available,
			Demo:      slice.Status.Demo,
			Reason:    slice.Status.Reason,
			Hours:     len(slice.Hours),
			Fields:    models.FieldsOfKind(slice.Kind),
		}
		if len(slice.Hours) > 0 {
			item.From = slice.Hours[0].Hour.UTC().Format(time.RFC3339)
			item.To = slice.ValidUntil.UTC().Format(time.RFC3339)
		}
		coverage = append(coverage, item)
	}

	return coverage
}

// CellGroup lists places that share one model grid cell. Sharing a cell means
// the provider returns the same weather for them regardless of terrain.
type CellGroup struct {
	Cell   string
	Places []string
}

// CoverageReport answers "what data can we actually get, and for which points".
type CoverageReport struct {
	Day         string
	HorizonDays int
	Probes      []CoverageProbe
	SharedCells []CellGroup
	Missing     []PlaceFailure
	StartedAt   time.Time
	FinishedAt  time.Time
}

func (r CoverageReport) CellCount() int {
	cells := make(map[string]struct{}, len(r.Probes))
	for _, probe := range r.Probes {
		if key := probe.CellKey(); key != "" {
			cells[key] = struct{}{}
		}
	}

	return len(cells)
}

// CellKey identifies the model cell that produced the probe.
func (p CoverageProbe) CellKey() string {
	if !p.Geometry.ModelLatitude.Known || !p.Geometry.ModelLongitude.Known {
		return ""
	}

	return fmt.Sprintf("%.3f,%.3f", p.Geometry.ModelLatitude.Value, p.Geometry.ModelLongitude.Value)
}

const sharedCellThreshold = 2

// ProbeCoverageUseCase fetches one day of data for every place and reports
// which sources and fields actually arrived. It is a diagnostics command: it
// validates the pipeline across all map points, not just the seed ones.
type ProbeCoverageUseCase struct {
	places   placeReader
	provider weatherProvider
}

func NewProbeCoverageUseCase(places placeReader, provider weatherProvider) *ProbeCoverageUseCase {
	return &ProbeCoverageUseCase{places: places, provider: provider}
}

func (u *ProbeCoverageUseCase) Do(
	ctx context.Context,
	rng models.ForecastRange,
	placeID string,
) (CoverageReport, error) {
	report := CoverageReport{
		Day:         rng.Today.Date(),
		HorizonDays: len(rng.Days()),
		StartedAt:   time.Now().UTC(),
	}

	places, err := selectPlaces(ctx, u.places, placeID)
	if err != nil {
		return report, err
	}

	for _, place := range places {
		probe := CoverageProbe{
			PlaceID:     place.ID,
			PlaceName:   place.Name,
			PlaceType:   place.Type,
			Layer:       place.Layer,
			Elevation:   place.Elevation,
			FieldCounts: make(map[models.Factor]int),
		}

		series, fErr := u.provider.FetchHourly(ctx, place, rng)
		if fErr != nil {
			probe.Error = fErr.Error()
			report.Probes = append(report.Probes, probe)
			report.Missing = append(report.Missing, PlaceFailure{
				PlaceID:   place.ID,
				PlaceName: place.Name,
				Reason:    fErr.Error(),
			})

			continue
		}

		probe.Sources = series.Sources
		probe.Geometry = series.Geometry
		probe.Slices = series.Slices
		probe.Hours = len(series.Hours)
		countFields(&probe, series.Hours)
		report.Probes = append(report.Probes, probe)
	}

	report.SharedCells = groupSharedCells(report.Probes)
	report.FinishedAt = time.Now().UTC()

	return report, nil
}

func countFields(probe *CoverageProbe, hours []models.WeatherObservation) {
	factors := []models.Factor{
		models.FactorAirTemperature,
		models.FactorWindSpeed,
		models.FactorWindGust,
		models.FactorPrecipitation,
		models.FactorAirQuality,
		models.FactorWaveHeight,
		models.FactorWaterTemperature,
		models.FactorVisibility,
	}
	for _, obs := range hours {
		for _, factor := range factors {
			if obs.Field(factor).Known {
				probe.FieldCounts[factor]++
			}
		}
	}
}

func groupSharedCells(probes []CoverageProbe) []CellGroup {
	byCell := make(map[string][]string)
	order := make([]string, 0)
	for _, probe := range probes {
		key := probe.CellKey()
		if key == "" {
			continue
		}
		if _, exists := byCell[key]; !exists {
			order = append(order, key)
		}
		byCell[key] = append(byCell[key], probe.PlaceName)
	}

	groups := make([]CellGroup, 0)
	for _, key := range order {
		if len(byCell[key]) < sharedCellThreshold {
			continue
		}
		groups = append(groups, CellGroup{Cell: key, Places: byCell[key]})
	}

	return groups
}
