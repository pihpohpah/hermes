package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

func TestProbeCoverageGroupsPlacesSharingOneModelCell(t *testing.T) {
	t.Parallel()

	window, err := models.NewDayWindow(2026, time.November, 13)
	if err != nil {
		t.Fatalf("window: %v", err)
	}
	rng, err := models.NewForecastRange(window.Start)
	if err != nil {
		t.Fatalf("range: %v", err)
	}

	places := []models.Place{
		{ID: "11111111-1111-1111-1111-111111111111", Name: "Нагорный парк", Type: models.PlaceTypePark},
		{ID: "22222222-2222-2222-2222-222222222222", Name: "Сопка Бурачка", Type: models.PlaceTypeViewpoint},
		{ID: "33333333-3333-3333-3333-333333333333", Name: "Маяк", Type: models.PlaceTypeSupSpot},
	}

	provider := &fixedGeometryProvider{
		series: seriesFor(window),
		cells: map[string]models.ModelGeometry{
			"Нагорный парк": {
				ModelLatitude:  models.NewMeasure(43.128),
				ModelLongitude: models.NewMeasure(131.982),
			},
			"Сопка Бурачка": {
				ModelLatitude:  models.NewMeasure(43.128),
				ModelLongitude: models.NewMeasure(131.982),
			},
			"Маяк": {
				ModelLatitude:  models.NewMeasure(42.988),
				ModelLongitude: models.NewMeasure(131.857),
			},
		},
	}

	uc := usecase.NewProbeCoverageUseCase(&fakePlaces{places: places}, provider)
	report, err := uc.Do(context.Background(), rng, "")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}

	if len(report.Probes) != 3 {
		t.Fatalf("probes = %d, want 3", len(report.Probes))
	}
	if report.CellCount() != 2 {
		t.Fatalf("model cells = %d, want 2", report.CellCount())
	}
	if len(report.SharedCells) != 1 {
		t.Fatalf("shared cells = %d, want 1", len(report.SharedCells))
	}
	if len(report.SharedCells[0].Places) != 2 {
		t.Fatalf("shared cell places = %d, want 2", len(report.SharedCells[0].Places))
	}
}

func TestProbeCoverageReportsFieldAvailability(t *testing.T) {
	t.Parallel()

	window, err := models.NewDayWindow(2026, time.November, 13)
	if err != nil {
		t.Fatalf("window: %v", err)
	}
	rng, err := models.NewForecastRange(window.Start)
	if err != nil {
		t.Fatalf("range: %v", err)
	}

	places := []models.Place{
		{ID: "11111111-1111-1111-1111-111111111111", Name: "Маяк", Type: models.PlaceTypeSupSpot},
	}
	uc := usecase.NewProbeCoverageUseCase(&fakePlaces{places: places}, &fakeProvider{
		series: seriesFor(window),
	})

	report, err := uc.Do(context.Background(), rng, "")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if len(report.Probes) != 1 {
		t.Fatalf("probes = %d, want 1", len(report.Probes))
	}

	probe := report.Probes[0]
	if probe.Hours != window.Hours() {
		t.Fatalf("hours = %d, want %d", probe.Hours, window.Hours())
	}
	if probe.FieldCounts[models.FactorWaveHeight] != window.Hours() {
		t.Fatalf("wave height availability = %d, want %d",
			probe.FieldCounts[models.FactorWaveHeight], window.Hours())
	}
}

type fixedGeometryProvider struct {
	series models.WeatherSeries
	cells  map[string]models.ModelGeometry
}

func (f *fixedGeometryProvider) FetchSource(
	context.Context,
	models.Place,
	models.SourceKind,
	models.ForecastRange,
) (models.SourceSlice, error) {
	return models.SourceSlice{}, nil
}

func (f *fixedGeometryProvider) FetchHourly(
	_ context.Context,
	place models.Place,
	_ models.ForecastRange,
) (models.WeatherSeries, error) {
	series := f.series
	series.Geometry = f.cells[place.Name]

	return series, nil
}
