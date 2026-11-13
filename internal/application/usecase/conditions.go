package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
	"github.com/pihpohpah/hermes/internal/domain/scoring"
)

const (
	maxPlacesPerRun = 500
	// rangeSlack tolerates a rolling horizon: the first stored hour can be a bit
	// older than the freshly computed lower bound.
	rangeSlack = 2 * time.Hour
)

type placeReader interface {
	List(ctx context.Context, filter models.PlaceFilter) ([]models.Place, error)
	GetByID(ctx context.Context, id string) (models.Place, error)
}

type weatherProvider interface {
	FetchHourly(ctx context.Context, place models.Place, rng models.ForecastRange) (models.WeatherSeries, error)
	FetchSource(
		ctx context.Context,
		place models.Place,
		kind models.SourceKind,
		rng models.ForecastRange,
	) (models.SourceSlice, error)
}

type weatherStore interface {
	UpsertSeries(ctx context.Context, series models.WeatherSeries) (int, error)
	ListSeries(ctx context.Context, placeID string, from, to time.Time) (models.StoredSeries, error)
}

type assessmentStore interface {
	UpsertBatch(ctx context.Context, items []models.ConditionAssessment) error
	ListByPlaceAndRange(
		ctx context.Context,
		placeID string,
		activity models.Activity,
		from time.Time,
		to time.Time,
	) ([]models.ConditionAssessment, error)
}

// conditionsService is the shared application step: read (or fetch) hourly
// observations and run the domain scorer. Use cases compose it so weather
// loading and scoring never mix responsibilities.
type conditionsService struct {
	weather  weatherStore
	provider weatherProvider
}

// observations returns stored hours for the range, fetching and storing the
// whole horizon when the cache does not cover it.
func (s conditionsService) observations(
	ctx context.Context,
	place models.Place,
	rng models.ForecastRange,
) (models.StoredSeries, error) {
	stored, err := s.weather.ListSeries(ctx, place.ID, rng.From, rng.To)
	if err != nil {
		return models.StoredSeries{}, fmt.Errorf("read weather for place `%s`: %w", place.ID, err)
	}
	if len(stored.Observations) > 0 && coversRange(stored.Observations, rng) {
		return stored, nil
	}

	series, err := s.provider.FetchHourly(ctx, place, rng)
	if err != nil {
		return models.StoredSeries{}, fmt.Errorf("fetch weather for place `%s`: %w", place.ID, err)
	}
	if _, err := s.weather.UpsertSeries(ctx, series); err != nil {
		return models.StoredSeries{}, fmt.Errorf("store weather for place `%s`: %w", place.ID, err)
	}

	stored, err = s.weather.ListSeries(ctx, place.ID, rng.From, rng.To)
	if err != nil {
		return models.StoredSeries{}, fmt.Errorf("reread weather for place `%s`: %w", place.ID, err)
	}
	stored.Geometry = series.Geometry
	// The fetch just told us the truth about each source; stale status rows must
	// not claim a source is available when this attempt failed.
	stored.Sources = series.Sources
	stored.Coverage = coverageOf(series)
	if len(stored.Observations) == 0 {
		stored.Observations = series.Hours
	}

	return stored, nil
}

// coversRange reports whether stored hours already span the requested range.
func coversRange(observations []models.WeatherObservation, rng models.ForecastRange) bool {
	if len(observations) == 0 {
		return false
	}
	first := observations[0].Hour
	last := observations[len(observations)-1].Hour

	return !first.After(rng.From.Add(rangeSlack)) && !last.Before(rng.To.Add(-2*rangeSlack))
}

func (s conditionsService) assess(
	placeID string,
	activity models.Activity,
	observations []models.WeatherObservation,
	sources []models.SourceStatus,
	geometry models.ModelGeometry,
	coverage []models.SourceCoverage,
) ([]models.ConditionAssessment, error) {
	demo := hasDemoSource(sources)
	assessments := make([]models.ConditionAssessment, 0, len(observations))
	for _, obs := range observations {
		assessment, err := scoring.Evaluate(activity, placeID, obs, sources, demo)
		if err != nil {
			return nil, fmt.Errorf("assess place `%s` for `%s`: %w", placeID, activity, err)
		}
		assessment.Geometry = geometry
		assessment.Coverage = coverage
		assessments = append(assessments, assessment)
	}

	return assessments, nil
}

func hasDemoSource(sources []models.SourceStatus) bool {
	for _, source := range sources {
		if source.Demo {
			return true
		}
	}

	return false
}

func selectPlaces(
	ctx context.Context,
	places placeReader,
	placeID string,
) ([]models.Place, error) {
	if placeID == "" {
		return places.List(ctx, models.PlaceFilter{Limit: maxPlacesPerRun})
	}

	validID, err := models.ValidateUUID("place_id", placeID)
	if err != nil {
		return nil, err
	}
	place, err := places.GetByID(ctx, validID)
	if err != nil {
		return nil, err
	}

	return []models.Place{place}, nil
}

type PlaceFailure struct {
	PlaceID   string
	PlaceName string
	Reason    string
}

// LoadWeatherUseCase downloads hourly weather and stores raw observations. It
// deliberately does not score anything.
type LoadWeatherUseCase struct {
	places   placeReader
	weather  weatherStore
	provider weatherProvider
}

func NewLoadWeatherUseCase(places placeReader, weather weatherStore, provider weatherProvider) *LoadWeatherUseCase {
	return &LoadWeatherUseCase{places: places, weather: weather, provider: provider}
}

type WeatherLoadReport struct {
	Day          string
	LoadedPlaces int
	LoadedHours  int
	HorizonDays  int
	From         time.Time
	To           time.Time
	Coverage     []models.SourceCoverage
	Failed       []PlaceFailure
	StartedAt    time.Time
	FinishedAt   time.Time
}

// Do loads the rolling horizon: current hours plus the forecast ahead.
func (u *LoadWeatherUseCase) Do(
	ctx context.Context,
	rng models.ForecastRange,
	placeID string,
) (WeatherLoadReport, error) {
	report := WeatherLoadReport{
		Day:         rng.Today.Date(),
		HorizonDays: len(rng.Days()),
		From:        rng.From,
		To:          rng.To,
		StartedAt:   time.Now().UTC(),
	}

	places, err := selectPlaces(ctx, u.places, placeID)
	if err != nil {
		return report, err
	}

	for _, place := range places {
		series, fErr := u.provider.FetchHourly(ctx, place, rng)
		if fErr != nil {
			report.Failed = append(report.Failed, PlaceFailure{
				PlaceID:   place.ID,
				PlaceName: place.Name,
				Reason:    fErr.Error(),
			})

			continue
		}

		hours, sErr := u.weather.UpsertSeries(ctx, series)
		if sErr != nil {
			return report, fmt.Errorf("store weather for place `%s`: %w", place.ID, sErr)
		}
		report.LoadedPlaces++
		report.LoadedHours += hours
		report.Coverage = coverageOf(series)
	}

	report.FinishedAt = time.Now().UTC()
	if len(places) > 0 && report.LoadedPlaces == 0 {
		return report, fmt.Errorf("weather loading failed for all %d places", len(places))
	}

	return report, nil
}

// AssessConditionsUseCase scores stored observations for the activities and
// persists explainable assessments.
type AssessConditionsUseCase struct {
	places     placeReader
	store      assessmentStore
	conditions conditionsService
}

func NewAssessConditionsUseCase(
	places placeReader,
	store assessmentStore,
	weather weatherStore,
	provider weatherProvider,
) *AssessConditionsUseCase {
	return &AssessConditionsUseCase{
		places:     places,
		store:      store,
		conditions: conditionsService{weather: weather, provider: provider},
	}
}

type AssessReport struct {
	Day         string
	Assessed    int
	Hours       int
	Assessments int
	Skipped     []PlaceFailure
	Activities  []models.Activity
	FinishedAt  time.Time
}

// Do scores the whole rolling horizon for the requested activities, so the API
// serves current conditions and the 10-day outlook from one dataset.
func (u *AssessConditionsUseCase) Do(
	ctx context.Context,
	rng models.ForecastRange,
	activities []models.Activity,
	placeID string,
) (AssessReport, error) {
	report := AssessReport{Day: rng.Today.Date(), Hours: rng.Hours()}

	activities, err := normalizeActivities(activities)
	if err != nil {
		return report, err
	}
	report.Activities = activities

	places, err := selectPlaces(ctx, u.places, placeID)
	if err != nil {
		return report, err
	}

	for _, place := range places {
		stored, oErr := u.conditions.observations(ctx, place, rng)
		if oErr != nil {
			report.Skipped = append(report.Skipped, PlaceFailure{
				PlaceID:   place.ID,
				PlaceName: place.Name,
				Reason:    oErr.Error(),
			})

			continue
		}

		items := make([]models.ConditionAssessment, 0, len(stored.Observations)*len(activities))
		for _, activity := range activities {
			assessed, aErr := u.conditions.assess(
				place.ID, activity, stored.Observations, stored.Sources,
				stored.Geometry, stored.Coverage,
			)
			if aErr != nil {
				return report, aErr
			}
			items = append(items, assessed...)
		}

		if sErr := u.store.UpsertBatch(ctx, items); sErr != nil {
			return report, fmt.Errorf("store assessments for place `%s`: %w", place.ID, sErr)
		}
		report.Assessed++
		report.Assessments += len(items)
	}

	report.FinishedAt = time.Now().UTC()

	return report, nil
}

func normalizeActivities(activities []models.Activity) ([]models.Activity, error) {
	if len(activities) == 0 {
		activities = []models.Activity{
			models.ActivityWalk,
			models.ActivitySup,
			models.ActivityHike,
			models.ActivityRun,
		}
	}

	seen := make(map[models.Activity]struct{}, len(activities))
	result := make([]models.Activity, 0, len(activities))
	for _, activity := range activities {
		if !activity.Valid() {
			return nil, models.Invalid("unexpected activity `" + string(activity) + "`")
		}
		if _, exists := seen[activity]; exists {
			continue
		}
		if _, err := scoring.ProfileFor(activity); err != nil {
			return nil, err
		}
		seen[activity] = struct{}{}
		result = append(result, activity)
	}

	return result, nil
}

// coverageOf builds per-source coverage from a freshly fetched series.
func coverageOf(series models.WeatherSeries) []models.SourceCoverage {
	coverage := make([]models.SourceCoverage, 0, len(series.Slices))
	for _, slice := range series.Slices {
		item := models.SourceCoverage{
			Kind:      slice.Kind,
			Available: slice.Status.Available,
			Demo:      slice.Status.Demo,
			Reason:    slice.Status.Reason,
			Hours:     len(slice.Hours),
			Covered:   countMeasured(slice),
			Fields:    models.FieldsOfKind(slice.Kind),
			FetchedAt: slice.FetchedAt.UTC().Format(time.RFC3339),
		}
		if len(slice.Hours) > 0 {
			item.From = slice.Hours[0].Hour.UTC().Format(time.RFC3339)
			item.To = slice.ValidUntil.UTC().Format(time.RFC3339)
		}
		coverage = append(coverage, item)
	}

	return coverage
}

func countMeasured(slice models.SourceSlice) int {
	count := 0
	for _, obs := range slice.Hours {
		for _, factor := range models.FieldsOfKind(slice.Kind) {
			if obs.Field(factor).Known {
				count++

				break
			}
		}
	}

	return count
}
