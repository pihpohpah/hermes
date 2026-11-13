package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// GetPlaceForecastUseCase returns hourly assessments for one place, activity
// and calendar day. The day is interpreted in Asia/Vladivostok (UTC+10), so its
// window starts at 14:00 UTC of the previous UTC day.
type GetPlaceForecastUseCase struct {
	places     placeReader
	store      assessmentStore
	conditions conditionsService
}

func NewGetPlaceForecastUseCase(
	places placeReader,
	store assessmentStore,
	weather weatherStore,
	provider weatherProvider,
) *GetPlaceForecastUseCase {
	return &GetPlaceForecastUseCase{
		places:     places,
		store:      store,
		conditions: conditionsService{weather: weather, provider: provider},
	}
}

func (u *GetPlaceForecastUseCase) Do(
	ctx context.Context,
	in models.ForecastQuery,
) ([]models.ConditionAssessment, error) {
	validPlaceID, err := models.ValidateUUID("place_id", in.PlaceID)
	if err != nil {
		return nil, err
	}

	activity := in.Activity
	if activity == "" {
		activity = models.ActivityWalk
	}
	if !activity.Valid() {
		return nil, models.Invalid("unexpected activity `" + string(activity) + "`")
	}

	place, err := u.places.GetByID(ctx, validPlaceID)
	if err != nil {
		return nil, err
	}

	// The requested day selects one calendar day out of the loaded horizon; the
	// range itself is rolling so scores stay current.
	window, err := models.CurrentDayWindow(in.Date)
	if err != nil {
		return nil, err
	}
	// The horizon is always anchored to "now": the provider only serves ~16 days
	// ahead, so a range built from a far future date would fall outside it.
	rng, err := models.NewForecastRange(time.Now())
	if err != nil {
		return nil, err
	}

	// Days > 1 returns the multi-day outlook (up to the loaded horizon).
	days := max(in.Days, 1)
	days = min(days, models.ForecastHorizonDays)
	rangeEnd := window.End
	if days > 1 {
		lastDay := window.Day.AddDate(0, 0, days-1)
		endWindow, wErr := models.NewDayWindow(lastDay.Year(), lastDay.Month(), lastDay.Day())
		if wErr != nil {
			return nil, wErr
		}
		rangeEnd = endWindow.End
	}

	cached, err := u.store.ListByPlaceAndRange(ctx, place.ID, activity, window.Start, rangeEnd)
	if err != nil {
		return nil, fmt.Errorf("read forecast cache: %w", err)
	}
	stored, err := u.conditions.observations(ctx, place, rng)
	if err != nil {
		return nil, err
	}

	// Geometry may be absent in rows written before it existed; fill it from
	// the observations instead of dropping the coarse-model caveat.
	if len(cached) > 0 && len(cached) >= days*window.Hours() {
		return backfillGeometry(cached, stored.Geometry), nil
	}

	fresh, err := u.conditions.assess(
		place.ID, activity, stored.Observations, stored.Sources,
		stored.Geometry, stored.Coverage,
	)
	if err != nil {
		return nil, err
	}
	if err := u.store.UpsertBatch(ctx, fresh); err != nil {
		return nil, fmt.Errorf("save forecast cache: %w", err)
	}

	return filterRange(fresh, window.Start, rangeEnd), nil
}

// filterRange keeps only the assessments inside [from, to).
func filterRange(
	items []models.ConditionAssessment,
	from time.Time,
	to time.Time,
) []models.ConditionAssessment {
	result := make([]models.ConditionAssessment, 0, len(items))
	for _, item := range items {
		if !item.ForecastTime.Before(from) && item.ForecastTime.Before(to) {
			result = append(result, item)
		}
	}

	return result
}

func backfillGeometry(
	items []models.ConditionAssessment,
	geometry models.ModelGeometry,
) []models.ConditionAssessment {
	if !geometry.ModelLatitude.Known && !geometry.PlaceElevationM.Known {
		return items
	}
	for i := range items {
		if items[i].Geometry.ModelLatitude.Known || items[i].Geometry.PlaceElevationM.Known {
			continue
		}
		items[i].Geometry = geometry
	}

	return items
}

// RefreshForecastUseCase reloads weather and recomputes assessments for all
// places, the requested day and the requested activities.
type RefreshForecastUseCase struct {
	loader *LoadWeatherUseCase
	assess *AssessConditionsUseCase
}

func NewRefreshForecastUseCase(
	places placeReader,
	store assessmentStore,
	weather weatherStore,
	provider weatherProvider,
) *RefreshForecastUseCase {
	return &RefreshForecastUseCase{
		loader: NewLoadWeatherUseCase(places, weather, provider),
		assess: NewAssessConditionsUseCase(places, store, weather, provider),
	}
}

type RefreshReport struct {
	Day         string
	HorizonDays int
	Coverage    []models.SourceCoverage
	LoadedHours int
	Assessed    int
	Assessments int
	Skipped     []PlaceFailure
	FinishedAt  string
}

func (u *RefreshForecastUseCase) Do(
	ctx context.Context,
	rng models.ForecastRange,
	activities []models.Activity,
) (RefreshReport, error) {
	loadReport, err := u.loader.Do(ctx, rng, "")
	if err != nil {
		return RefreshReport{Day: rng.Today.Date()}, fmt.Errorf("load weather for refresh: %w", err)
	}

	assessReport, err := u.assess.Do(ctx, rng, activities, "")
	if err != nil {
		return RefreshReport{Day: rng.Today.Date()}, fmt.Errorf("assess conditions for refresh: %w", err)
	}

	return RefreshReport{
		Day:         rng.Today.Date(),
		HorizonDays: len(rng.Days()),
		Coverage:    loadReport.Coverage,
		LoadedHours: loadReport.LoadedHours,
		Assessed:    assessReport.Assessed,
		Assessments: assessReport.Assessments,
		Skipped:     assessReport.Skipped,
		FinishedAt:  assessReport.FinishedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}
