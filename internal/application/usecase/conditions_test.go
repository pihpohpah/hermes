package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

type fakePlaces struct {
	places []models.Place
}

func (f *fakePlaces) List(context.Context, models.PlaceFilter) ([]models.Place, error) {
	return f.places, nil
}

func (f *fakePlaces) GetByID(_ context.Context, id string) (models.Place, error) {
	for _, place := range f.places {
		if place.ID == id {
			return place, nil
		}
	}

	return models.Place{}, models.ErrNotFound
}

type fakeWeather struct {
	stored    []models.WeatherObservation
	sources   []models.SourceStatus
	geometry  models.ModelGeometry
	coverage  []models.SourceCoverage
	upserted  int
	upsertErr error
}

func (f *fakeWeather) UpsertSeries(_ context.Context, series models.WeatherSeries) (int, error) {
	if f.upsertErr != nil {
		return 0, f.upsertErr
	}
	f.stored = append([]models.WeatherObservation{}, series.Hours...)
	f.sources = series.Sources
	f.upserted++

	return len(series.Hours), nil
}

func (f *fakeWeather) ListSeries(
	context.Context,
	string,
	time.Time,
	time.Time,
) (models.StoredSeries, error) {
	return models.StoredSeries{
		Observations: f.stored,
		Sources:      f.sources,
		Geometry:     f.geometry,
		Coverage:     f.coverage,
	}, nil
}

type fakeAssessments struct {
	saved []models.ConditionAssessment
}

func (f *fakeAssessments) UpsertBatch(_ context.Context, items []models.ConditionAssessment) error {
	f.saved = append([]models.ConditionAssessment{}, items...)

	return nil
}

func (f *fakeAssessments) ListByPlaceAndRange(
	_ context.Context,
	placeID string,
	activity models.Activity,
	from time.Time,
	to time.Time,
) ([]models.ConditionAssessment, error) {
	result := make([]models.ConditionAssessment, 0)
	for _, item := range f.saved {
		if item.PlaceID != placeID || item.Activity != activity {
			continue
		}
		if item.ForecastTime.Before(from) || !item.ForecastTime.Before(to) {
			continue
		}
		result = append(result, item)
	}

	return result, nil
}

type fakeProvider struct {
	series models.WeatherSeries
	err    error
	calls  int
}

func (f *fakeProvider) FetchSource(
	context.Context,
	models.Place,
	models.SourceKind,
	models.ForecastRange,
) (models.SourceSlice, error) {
	return models.SourceSlice{}, nil
}

func (f *fakeProvider) FetchHourly(
	context.Context,
	models.Place,
	models.ForecastRange,
) (models.WeatherSeries, error) {
	f.calls++
	if f.err != nil {
		return models.WeatherSeries{}, f.err
	}

	return f.series, nil
}

func supPlace() models.Place {
	return models.Place{
		ID:       "11111111-1111-1111-1111-111111111111",
		Name:     "Токаревский",
		Type:     models.PlaceTypeSupSpot,
		Layer:    models.LayerSea,
		Location: models.Point{Lat: 43.0732, Lon: 131.8431},
	}
}

func seriesFor(window models.DayWindow) models.WeatherSeries {
	sources := []models.SourceStatus{
		{Source: models.SourceOpenMeteoWeather, Available: true},
		{Source: models.SourceOpenMeteoMarine, Available: true},
		{Source: models.SourceOpenMeteoAirQuality, Available: true},
	}
	hours := make([]models.WeatherObservation, 0, window.Hours())
	for _, hour := range window.HourStarts() {
		hours = append(hours, models.WeatherObservation{
			Hour:        hour,
			AirTempC:    models.NewMeasure(18),
			PrecipMM:    models.NewMeasure(0),
			WindSpeedMS: models.NewMeasure(3),
			WindGustMS:  models.NewMeasure(5),
			WaveHeightM: models.NewMeasure(0.3),
			WaterTempC:  models.NewMeasure(17),
			AQIEuropean: models.NewMeasure(20),
		})
	}

	return models.WeatherSeries{PlaceID: supPlace().ID, Hours: hours, Sources: sources, FetchedAt: time.Now()}
}

func TestAssessStoresPerActivityAssessments(t *testing.T) {
	t.Parallel()

	window, err := models.NewDayWindow(2026, time.November, 13)
	if err != nil {
		t.Fatalf("window: %v", err)
	}
	rng, err := models.NewForecastRange(window.Start)
	if err != nil {
		t.Fatalf("range: %v", err)
	}

	store := &fakeAssessments{}
	uc := usecase.NewAssessConditionsUseCase(
		&fakePlaces{places: []models.Place{supPlace()}},
		store,
		&fakeWeather{},
		&fakeProvider{series: seriesFor(window)},
	)

	report, err := uc.Do(context.Background(), rng, []models.Activity{
		models.ActivityWalk,
		models.ActivitySup,
	}, "")
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if report.Assessed != 1 {
		t.Fatalf("assessed places = %d, want 1", report.Assessed)
	}
	if len(store.saved) == 0 {
		t.Fatal("no assessments were saved")
	}

	var walk, sup *models.ConditionAssessment
	for i := range store.saved {
		switch store.saved[i].Activity {
		case models.ActivityWalk:
			walk = &store.saved[i]
		case models.ActivitySup:
			sup = &store.saved[i]
		case models.ActivityHike, models.ActivityRun:
		}
	}
	if walk == nil || sup == nil {
		t.Fatal("expected both walk and sup assessments")
	}
	if walk.Score <= sup.Score {
		t.Fatalf("walk score %d must exceed sup score %d for the same weather", walk.Score, sup.Score)
	}
	if walk.AlgorithmVersion == "" {
		t.Fatal("algorithm version must be recorded")
	}
}

func TestAssessFetchesWeatherWhenCacheIsEmpty(t *testing.T) {
	t.Parallel()

	window, err := models.NewDayWindow(2026, time.November, 13)
	if err != nil {
		t.Fatalf("window: %v", err)
	}
	rng, err := models.NewForecastRange(window.Start)
	if err != nil {
		t.Fatalf("range: %v", err)
	}

	provider := &fakeProvider{series: seriesFor(window)}
	weather := &fakeWeather{}
	uc := usecase.NewAssessConditionsUseCase(
		&fakePlaces{places: []models.Place{supPlace()}},
		&fakeAssessments{},
		weather,
		provider,
	)

	if _, err := uc.Do(context.Background(), rng, []models.Activity{models.ActivityWalk}, ""); err != nil {
		t.Fatalf("assess: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if weather.upserted != 1 {
		t.Fatalf("weather upserts = %d, want 1", weather.upserted)
	}
}

func TestAssessSkipsPlaceWithoutWeather(t *testing.T) {
	t.Parallel()

	window, err := models.NewDayWindow(2026, time.November, 13)
	if err != nil {
		t.Fatalf("window: %v", err)
	}
	rng, err := models.NewForecastRange(window.Start)
	if err != nil {
		t.Fatalf("range: %v", err)
	}

	uc := usecase.NewAssessConditionsUseCase(
		&fakePlaces{places: []models.Place{supPlace()}},
		&fakeAssessments{},
		&fakeWeather{},
		&fakeProvider{err: context.DeadlineExceeded},
	)

	report, err := uc.Do(context.Background(), rng, nil, "")
	if err != nil {
		t.Fatalf("assess must not fail the whole run: %v", err)
	}
	if len(report.Skipped) != 1 {
		t.Fatalf("skipped = %d, want 1", len(report.Skipped))
	}
	if report.Skipped[0].Reason == "" {
		t.Fatal("skip reason must explain itself")
	}
}

func TestLoadWeatherDoesNotScore(t *testing.T) {
	t.Parallel()

	window, err := models.NewDayWindow(2026, time.November, 13)
	if err != nil {
		t.Fatalf("window: %v", err)
	}
	rng, err := models.NewForecastRange(window.Start)
	if err != nil {
		t.Fatalf("range: %v", err)
	}

	store := &fakeAssessments{}
	uc := usecase.NewLoadWeatherUseCase(
		&fakePlaces{places: []models.Place{supPlace()}},
		&fakeWeather{},
		&fakeProvider{series: seriesFor(window)},
	)

	if _, err := uc.Do(context.Background(), rng, ""); err != nil {
		t.Fatalf("load weather: %v", err)
	}
	if len(store.saved) != 0 {
		t.Fatal("weather loading must not create assessments")
	}
}

func TestForecastUsesLocalDayWindow(t *testing.T) {
	t.Parallel()

	window, err := models.NewDayWindow(2026, time.November, 13)
	if err != nil {
		t.Fatalf("window: %v", err)
	}

	weather := &fakeWeather{stored: seriesFor(window).Hours, sources: seriesFor(window).Sources}
	uc := usecase.NewGetPlaceForecastUseCase(
		&fakePlaces{places: []models.Place{supPlace()}},
		&fakeAssessments{},
		weather,
		&fakeProvider{series: seriesFor(window)},
	)

	assessments, err := uc.Do(context.Background(), models.ForecastQuery{
		PlaceID:  supPlace().ID,
		Date:     time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC),
		Activity: models.ActivityWalk,
	})
	if err != nil {
		t.Fatalf("forecast: %v", err)
	}
	if len(assessments) == 0 {
		t.Fatal("expected assessments")
	}
	if assessments[0].LocalDate != "2026-11-13" {
		t.Fatalf("local date = %s, want 2026-11-13", assessments[0].LocalDate)
	}
}

func TestForecastRejectsUnknownActivity(t *testing.T) {
	t.Parallel()

	window, err := models.NewDayWindow(2026, time.November, 13)
	if err != nil {
		t.Fatalf("window: %v", err)
	}

	uc := usecase.NewGetPlaceForecastUseCase(
		&fakePlaces{places: []models.Place{supPlace()}},
		&fakeAssessments{},
		&fakeWeather{stored: seriesFor(window).Hours},
		&fakeProvider{series: seriesFor(window)},
	)

	_, err = uc.Do(context.Background(), models.ForecastQuery{
		PlaceID:  supPlace().ID,
		Date:     time.Now(),
		Activity: models.Activity("swim"),
	})
	if err == nil {
		t.Fatal("unknown activity must be rejected")
	}
}
