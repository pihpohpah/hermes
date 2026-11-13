package openmeteo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

const (
	defaultTimeout       = 8 * time.Second
	defaultAttempts      = 3
	defaultBaseBackoff   = 300 * time.Millisecond
	defaultMaxBackoff    = 3 * time.Second
	sourceStatusCapacity = 3
	errorBodyLimit       = 200

	providerName = "open-meteo"

	// Open-Meteo returns wind speed in km/h unless requested otherwise.
	kmhPerMS = 3.6

	weatherHourlyFields = "temperature_2m,apparent_temperature,precipitation,wind_speed_10m," +
		"wind_gusts_10m,wind_direction_10m,relative_humidity_2m,cloud_cover,visibility,weather_code"
	marineHourlyFields = "wave_height,wave_period,wave_direction,sea_surface_temperature,sea_level_height_msl"
	airHourlyFields    = "pm2_5,european_aqi"

	maxResponseBytes = 4 << 20
)

// Config configures the real Open-Meteo provider. Zero values fall back to the
// public API and conservative network defaults.
type Config struct {
	WeatherBaseURL    string
	MarineBaseURL     string
	AirQualityBaseURL string
	Timeout           time.Duration
	Attempts          int
	BaseBackoff       time.Duration
	MaxBackoff        time.Duration
	HTTPClient        *http.Client
	APIKey            string
}

func (c Config) withDefaults() Config {
	if c.WeatherBaseURL == "" {
		c.WeatherBaseURL = "https://api.open-meteo.com/v1"
	}
	if c.MarineBaseURL == "" {
		c.MarineBaseURL = "https://marine-api.open-meteo.com/v1"
	}
	if c.AirQualityBaseURL == "" {
		c.AirQualityBaseURL = "https://air-quality-api.open-meteo.com/v1"
	}
	if c.Timeout <= 0 {
		c.Timeout = defaultTimeout
	}
	if c.Attempts <= 0 {
		c.Attempts = defaultAttempts
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = defaultBaseBackoff
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = defaultMaxBackoff
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: c.Timeout}
	}

	return c
}

// Client fetches hourly data from the Open-Meteo weather, marine and air
// quality APIs and joins everything by UTC timestamp.
type Client struct {
	cfg   Config
	now   func() time.Time
	sleep func(context.Context, time.Duration) error
}

func NewClient(cfg Config) *Client {
	return &Client{
		cfg:   cfg.withDefaults(),
		now:   time.Now,
		sleep: sleep,
	}
}

type hourlyResponse struct {
	Error     bool    `json:"error"`
	Reason    string  `json:"reason"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Elevation float64 `json:"elevation"`

	Hourly struct {
		Time                  []string   `json:"time"`
		Temperature2m         []*float64 `json:"temperature_2m"`
		ApparentTemperature   []*float64 `json:"apparent_temperature"`
		Precipitation         []*float64 `json:"precipitation"`
		WindSpeed10m          []*float64 `json:"wind_speed_10m"`
		WindGusts10m          []*float64 `json:"wind_gusts_10m"`
		WindDirection10m      []*float64 `json:"wind_direction_10m"`
		RelativeHumidity2m    []*float64 `json:"relative_humidity_2m"`
		CloudCover            []*float64 `json:"cloud_cover"`
		Visibility            []*float64 `json:"visibility"`
		WeatherCode           []*float64 `json:"weather_code"`
		WaveHeight            []*float64 `json:"wave_height"`
		WavePeriod            []*float64 `json:"wave_period"`
		WaveDirection         []*float64 `json:"wave_direction"`
		SeaSurfaceTemperature []*float64 `json:"sea_surface_temperature"`
		SeaLevelHeightMSL     []*float64 `json:"sea_level_height_msl"`
		PM25                  []*float64 `json:"pm2_5"`
		EuropeanAQI           []*float64 `json:"european_aqi"`
	} `json:"hourly"`
}

// FetchHourly fetches all three sources independently and merges them by UTC
// hour. A failing optional source (marine/AQI) never fails the forecast, and a
// source with a shorter horizon never truncates the others.
func (c *Client) FetchHourly(
	ctx context.Context,
	place models.Place,
	rng models.ForecastRange,
) (models.WeatherSeries, error) {
	slices := make([]models.SourceSlice, 0, sourceStatusCapacity)
	for _, kind := range models.AllSourceKinds() {
		slice, err := c.FetchSource(ctx, place, kind, rng)
		if err != nil {
			// Weather is the only required source.
			if kind == models.SourceKindWeather {
				return models.WeatherSeries{}, err
			}
			slice = unavailableSlice(kind, models.SourceIDForKind(kind), sourceReason(err))
		}
		slices = append(slices, slice)
	}

	series := models.MergeSlices(place.ID, slices)
	series.FetchedAt = c.now().UTC()
	if weather, ok := series.SliceFor(models.SourceKindWeather); ok {
		series.Geometry = weather.Geometry
	}

	return series, nil
}

// RequiresMarine reports whether an activity or place type depends on the sea.
func RequiresMarine(pt models.PlaceType) bool {
	switch pt {
	case models.PlaceTypeSupSpot, models.PlaceTypeBeach:
		return true
	case models.PlaceTypePark, models.PlaceTypeViewpoint, models.PlaceTypeTrailhead:
		return false
	default:
		return false
	}
}

// SetSleepForTest replaces the backoff sleeper so tests never wait for real.
func (c *Client) SetSleepForTest(fn func(context.Context, time.Duration) error) {
	c.sleep = fn
}

func (c *Client) fetch(
	ctx context.Context,
	endpoint string,
	hourlyFields string,
	params url.Values,
) (hourlyResponse, error) {
	query := url.Values{}
	for key, values := range params {
		query[key] = append(query[key], values...)
	}
	query.Set("hourly", hourlyFields)

	if c.cfg.APIKey != "" {
		query.Set("apikey", c.cfg.APIKey)
	}

	target := endpoint
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}

	var (
		response hourlyResponse
		err      error
	)
	for attempt := 1; attempt <= c.cfg.Attempts; attempt++ {
		response, err = c.doOnce(ctx, target)
		if err == nil {
			return response, nil
		}
		if !isRetryable(err) || attempt == c.cfg.Attempts {
			break
		}
		wait := c.backoff(attempt)
		if limited, ok := asRateLimited(err); ok {
			if limited.retryAfter > c.cfg.MaxBackoff {
				break
			}
			wait = max(wait, limited.retryAfter)
		}
		if waitErr := c.sleep(ctx, wait); waitErr != nil {
			return hourlyResponse{}, fmt.Errorf("retry open-meteo request: %w", waitErr)
		}
	}

	return hourlyResponse{}, err
}

func (c *Client) doOnce(ctx context.Context, target string) (hourlyResponse, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, target, nil)
	if err != nil {
		return hourlyResponse{}, fmt.Errorf("%w: build request: %w", ErrInvalidResponse, err)
	}
	req.Header.Set("User-Agent", "hermes-weather/"+providerName)
	req.Header.Set("Accept", "application/json")

	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return hourlyResponse{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if readErr != nil {
		return hourlyResponse{}, fmt.Errorf("%w: read body: %w", ErrUnavailable, readErr)
	}

	if resp.StatusCode != http.StatusOK {
		return hourlyResponse{}, c.statusFailure(resp, body)
	}

	var decoded hourlyResponse
	if decodeErr := json.Unmarshal(body, &decoded); decodeErr != nil {
		return hourlyResponse{}, fmt.Errorf("%w: decode body: %w", ErrInvalidResponse, decodeErr)
	}
	if decoded.Error {
		return hourlyResponse{}, &statusError{
			kind:       ErrInvalidResponse,
			statusCode: resp.StatusCode,
			reason:     decoded.Reason,
		}
	}

	return decoded, nil
}

func (c *Client) statusFailure(resp *http.Response, body []byte) error {
	status := resp.StatusCode

	if status == http.StatusTooManyRequests {
		return &statusError{
			kind:       ErrRateLimited,
			statusCode: status,
			reason:     "too many requests",
			retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), c.now()),
		}
	}

	kind := ErrInvalidResponse
	if status >= http.StatusInternalServerError {
		kind = ErrUnavailable
	}

	return &statusError{kind: kind, statusCode: status, reason: truncate(string(body), errorBodyLimit)}
}

func (c *Client) backoff(attempt int) time.Duration {
	if c.cfg.BaseBackoff <= 0 {
		return 0
	}
	wait := c.cfg.BaseBackoff << (attempt - 1)
	if wait > c.cfg.MaxBackoff || wait <= 0 {
		return c.cfg.MaxBackoff
	}

	return wait
}

func applyWeather(byHour map[time.Time]models.WeatherObservation, resp hourlyResponse) {
	for i, raw := range resp.Hourly.Time {
		hour, ok := parseHour(raw)
		if !ok {
			continue
		}
		obs, exists := byHour[hour]
		if !exists {
			continue
		}
		obs.AirTempC = measureAt(resp.Hourly.Temperature2m, i)
		obs.ApparentTempC = measureAt(resp.Hourly.ApparentTemperature, i)
		obs.PrecipMM = measureAt(resp.Hourly.Precipitation, i)
		obs.WindSpeedMS = divide(measureAt(resp.Hourly.WindSpeed10m, i), kmhPerMS)
		obs.WindGustMS = divide(measureAt(resp.Hourly.WindGusts10m, i), kmhPerMS)
		obs.WindDirection = measureAt(resp.Hourly.WindDirection10m, i)
		obs.HumidityPct = measureAt(resp.Hourly.RelativeHumidity2m, i)
		obs.CloudCoverPct = measureAt(resp.Hourly.CloudCover, i)
		obs.VisibilityM = measureAt(resp.Hourly.Visibility, i)
		obs.WeatherCode = measureAt(resp.Hourly.WeatherCode, i)
		byHour[hour] = obs
	}
}

func applyMarine(byHour map[time.Time]models.WeatherObservation, resp hourlyResponse) {
	for i, raw := range resp.Hourly.Time {
		hour, ok := parseHour(raw)
		if !ok {
			continue
		}
		obs, exists := byHour[hour]
		if !exists {
			continue
		}
		obs.WaveHeightM = measureAt(resp.Hourly.WaveHeight, i)
		obs.WavePeriodS = measureAt(resp.Hourly.WavePeriod, i)
		obs.WaveDirection = measureAt(resp.Hourly.WaveDirection, i)
		obs.WaterTempC = measureAt(resp.Hourly.SeaSurfaceTemperature, i)
		obs.SeaLevelHeightM = measureAt(resp.Hourly.SeaLevelHeightMSL, i)
		byHour[hour] = obs
	}
}

func applyAirQuality(byHour map[time.Time]models.WeatherObservation, resp hourlyResponse) {
	for i, raw := range resp.Hourly.Time {
		hour, ok := parseHour(raw)
		if !ok {
			continue
		}
		obs, exists := byHour[hour]
		if !exists {
			continue
		}
		obs.PM25 = measureAt(resp.Hourly.PM25, i)
		obs.AQIEuropean = measureAt(resp.Hourly.EuropeanAQI, i)
		byHour[hour] = obs
	}
}

func measureAt(values []*float64, index int) models.Measure {
	if index < 0 || index >= len(values) || values[index] == nil {
		return models.MissingMeasure()
	}

	return models.NewMeasure(*values[index])
}

func divide(m models.Measure, divisor float64) models.Measure {
	if !m.Known || divisor == 0 {
		return m
	}

	return models.NewMeasure(m.Value / divisor)
}

func parseHour(raw string) (time.Time, bool) {
	parsed, err := time.ParseInLocation("2006-01-02T15:04", raw, time.UTC)
	if err != nil {
		return time.Time{}, false
	}

	return parsed.UTC(), true
}

func parseRetryAfter(raw string, now time.Time) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(raw); err == nil {
		if delay := at.Sub(now); delay > 0 {
			return delay
		}
	}

	return 0
}

func isRetryable(err error) bool {
	return errors.Is(err, ErrUnavailable) || errors.Is(err, ErrRateLimited)
}

func asRateLimited(err error) (*statusError, bool) {
	var limited *statusError
	if errors.As(err, &limited) && errors.Is(limited.kind, ErrRateLimited) {
		return limited, true
	}

	return nil, false
}

func sourceReason(err error) string {
	switch {
	case errors.Is(err, ErrRateLimited):
		return "превышен лимит запросов"
	case errors.Is(err, ErrInvalidResponse):
		return "источник вернул некорректный ответ"
	case errors.Is(err, ErrUnavailable):
		return "источник временно недоступен"
	default:
		return "источник недоступен"
	}
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}

	return text[:limit] + "…"
}

func sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// placeElevation returns the stored altitude used to downscale the model.
func placeElevation(place models.Place) *int {
	if place.Elevation == nil {
		return nil
	}

	return place.Elevation
}

// buildGeometry captures the model cell and how far the real place is from it.
func buildGeometry(place models.Place, resp hourlyResponse) models.ModelGeometry {
	geometry := models.ModelGeometry{
		ModelLatitude:   models.NewMeasure(resp.Latitude),
		ModelLongitude:  models.NewMeasure(resp.Longitude),
		ModelElevationM: models.NewMeasure(resp.Elevation),
	}
	if resp.Latitude == 0 && resp.Longitude == 0 {
		geometry.ModelLatitude = models.MissingMeasure()
		geometry.ModelLongitude = models.MissingMeasure()
	}
	if resp.Elevation == 0 && place.Elevation != nil && *place.Elevation > 100 {
		// A zero model elevation for a mountainous place means the API did not
		// report it; keep it unknown rather than pretending it is sea level.
		geometry.ModelElevationM = models.MissingMeasure()
	}
	if place.Elevation == nil {
		return geometry
	}

	placeElevationM := models.NewMeasure(float64(*place.Elevation))
	geometry.PlaceElevationM = placeElevationM
	geometry.ElevationSource = string(place.ElevationSource)
	geometry.Terrain = string(models.ClassifyTerrain(*place.Elevation))
	if geometry.ModelElevationM.Known {
		geometry.LapseCorrectionC = models.NewMeasure(
			models.LapseCorrection(*place.Elevation, geometry.ModelElevationM.Value),
		)
	}

	return geometry
}
