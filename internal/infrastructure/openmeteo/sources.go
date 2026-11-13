package openmeteo

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// airQualityHorizonDays is the upstream cap for the air-quality API, counted
// inclusively from today (7 days total: today plus 6). Weather and marine reach
// farther, so a shared request window would silently shorten the whole forecast
// to the shortest source.
const airQualityHorizonDays = 7

// FetchSource fetches exactly one source for one place and range. Each source is
// independent: a marine failure or a shorter AQI horizon never degrades weather.
func (c *Client) FetchSource(
	ctx context.Context,
	place models.Place,
	kind models.SourceKind,
	rng models.ForecastRange,
) (models.SourceSlice, error) {
	switch kind {
	case models.SourceKindWeather:
		return c.fetchWeatherSource(ctx, place, rng)
	case models.SourceKindMarine:
		return c.fetchMarineSource(ctx, place, rng)
	case models.SourceKindAirQuality:
		return c.fetchAirQualitySource(ctx, place, rng)
	default:
		return models.SourceSlice{}, fmt.Errorf("%w: unexpected source `%s`", ErrInvalidResponse, kind)
	}
}

func (c *Client) fetchWeatherSource(
	ctx context.Context,
	place models.Place,
	rng models.ForecastRange,
) (models.SourceSlice, error) {
	params := c.rangeParams(place, rng)
	response, err := c.fetch(ctx, c.cfg.WeatherBaseURL+"/forecast", weatherHourlyFields, params)
	if err != nil {
		return models.SourceSlice{}, fmt.Errorf("fetch weather for place `%s`: %w", place.ID, err)
	}

	slice := newSourceSlice(models.SourceKindWeather, models.SourceSourceWeather, rng)
	applyWeather(slice.byHour, response)
	built := slice.build(hasMeasuredWeather)
	built.Geometry = buildGeometry(place, response)

	return built, nil
}

func (c *Client) fetchMarineSource(
	ctx context.Context,
	place models.Place,
	rng models.ForecastRange,
) (models.SourceSlice, error) {
	if !RequiresMarine(place.Type) && place.Layer != models.LayerSea {
		return skippedSlice(models.SourceKindMarine, models.SourceSourceMarine,
			"морские данные не запрашиваются: место не морское"), nil
	}

	params := c.rangeParams(place, rng)
	response, err := c.fetch(ctx, c.cfg.MarineBaseURL+"/marine", marineHourlyFields, params)
	if err != nil {
		return unavailableSlice(models.SourceKindMarine, models.SourceSourceMarine,
			"морской источник недоступен: "+sourceReason(err)), nil
	}

	slice := newSourceSlice(models.SourceKindMarine, models.SourceSourceMarine, rng)
	applyMarine(slice.byHour, response)

	return slice.build(hasMeasuredMarine), nil
}

func (c *Client) fetchAirQualitySource(
	ctx context.Context,
	place models.Place,
	rng models.ForecastRange,
) (models.SourceSlice, error) {
	params := c.rangeParams(place, rng)
	// AQI is capped upstream at 7 days counted from the UTC day of the request,
	// so the last allowed end_date is utcToday+6. Report the shorter coverage
	// instead of failing.
	utcNow := c.now().UTC()
	utcToday := time.Date(utcNow.Year(), utcNow.Month(), utcNow.Day(), 0, 0, 0, 0, time.UTC)
	lastAllowed := utcToday.AddDate(0, 0, airQualityHorizonDays-1)
	if clampDate(lastAllowed, rng).Before(rng.To.Add(-time.Hour)) {
		params.Set("end_date", lastAllowed.Format(time.DateOnly))
	}

	response, err := c.fetch(ctx, c.cfg.AirQualityBaseURL+"/air-quality", airHourlyFields, params)
	if err != nil {
		return unavailableSlice(models.SourceKindAirQuality, models.SourceSourceAirQuality,
			"качество воздуха недоступно: "+sourceReason(err)), nil
	}

	slice := newSourceSlice(models.SourceKindAirQuality, models.SourceSourceAirQuality, rng)
	applyAirQuality(slice.byHour, response)

	return slice.build(hasMeasuredAirQuality), nil
}

func clampDate(limit time.Time, rng models.ForecastRange) time.Time {
	upper := rng.To.Add(-time.Hour)
	if limit.After(upper) {
		return upper
	}

	return limit
}

func (c *Client) rangeParams(place models.Place, rng models.ForecastRange) url.Values {
	params := url.Values{}
	params.Set("latitude", strconv.FormatFloat(place.Location.Lat, 'f', 4, 64))
	params.Set("longitude", strconv.FormatFloat(place.Location.Lon, 'f', 4, 64))
	params.Set("timezone", "UTC")
	params.Set("start_date", rng.From.UTC().Format(time.DateOnly))
	params.Set("end_date", rng.To.Add(-time.Hour).UTC().Format(time.DateOnly))
	params.Set("timeformat", "iso8601")
	if elevation := placeElevation(place); elevation != nil {
		params.Set("elevation", strconv.Itoa(*elevation))
	}

	return params
}

// sliceBuilder accumulates one source's hourly values in range order.
type sliceBuilder struct {
	kind      models.SourceKind
	hourOrder []time.Time
	byHour    map[time.Time]models.WeatherObservation
	status    models.SourceStatus
	statusSet bool
}

func newSourceSlice(
	kind models.SourceKind,
	source models.SourceID,
	rng models.ForecastRange,
) *sliceBuilder {
	hours := max(rng.Hours(), 0)
	order := make([]time.Time, 0, hours)
	byHour := make(map[time.Time]models.WeatherObservation, hours)
	for instant := rng.From.UTC().Truncate(time.Hour); instant.Before(rng.To); instant = instant.Add(time.Hour) {
		order = append(order, instant)
		byHour[instant] = models.WeatherObservation{Hour: instant}
	}

	return &sliceBuilder{
		kind:      kind,
		hourOrder: order,
		byHour:    byHour,
		status:    models.SourceStatus{Source: source, Available: true},
		statusSet: true,
	}
}

func (b *sliceBuilder) build(filled func(models.WeatherObservation) bool) models.SourceSlice {
	hours := make([]models.WeatherObservation, 0, len(b.hourOrder))
	lastKnown := time.Time{}
	first := time.Time{}
	for _, instant := range b.hourOrder {
		obs := b.byHour[instant]
		if filled(obs) {
			if first.IsZero() {
				first = instant
			}
			lastKnown = instant
		}
		hours = append(hours, obs)
	}

	return models.SourceSlice{
		Kind:       b.kind,
		Status:     b.status,
		Hours:      hours,
		ValidUntil: lastKnown.Add(time.Hour),
		FetchedAt:  time.Now().UTC(),
	}
}

func unavailableSlice(kind models.SourceKind, source models.SourceID, reason string) models.SourceSlice {
	return models.SourceSlice{
		Kind: kind,
		Status: models.SourceStatus{
			Source:    source,
			Available: false,
			Reason:    reason,
		},
		ValidUntil: time.Now().UTC(),
		FetchedAt:  time.Now().UTC(),
	}
}

func skippedSlice(kind models.SourceKind, source models.SourceID, reason string) models.SourceSlice {
	return models.SourceSlice{
		Kind: kind,
		Status: models.SourceStatus{
			Source:    source,
			Available: false,
			Reason:    reason,
		},
		ValidUntil: time.Now().UTC(),
		FetchedAt:  time.Now().UTC(),
	}
}

func hasMeasuredWeather(obs models.WeatherObservation) bool {
	return obs.AirTempC.Known || obs.WindSpeedMS.Known
}

func hasMeasuredMarine(obs models.WeatherObservation) bool {
	return obs.WaveHeightM.Known || obs.WaterTempC.Known
}

func hasMeasuredAirQuality(obs models.WeatherObservation) bool {
	return obs.AQIEuropean.Known || obs.PM25.Known
}
