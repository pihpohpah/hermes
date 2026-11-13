package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

type WeatherRepository struct {
	pool *pgxpool.Pool
}

func NewWeatherRepository(pool *pgxpool.Pool) *WeatherRepository {
	return &WeatherRepository{pool: pool}
}

const observationColumns = `
	place_id::text,
	observed_at,
	source,
	air_temp_c,
	apparent_temp_c,
	precipitation_mm,
	wind_speed_ms,
	wind_gust_ms,
	wind_direction_deg,
	relative_humidity_pct,
	cloud_cover_pct,
	visibility_m,
	weather_code,
	wave_height_m,
	wave_period_s,
	wave_direction_deg,
	water_temp_c,
	sea_level_height_m,
	air_quality_index,
	pm2_5,
	model_geometry
`

// UpsertSeries stores every source independently. Each source keeps only its
// own columns, so a marine write never clears weather values and an AQI write
// never touches the sea.
func (r *WeatherRepository) UpsertSeries(ctx context.Context, series models.WeatherSeries) (int, error) {
	total := 0
	for _, slice := range series.Slices {
		written, err := r.upsertSourceSlice(ctx, series.PlaceID, slice)
		if err != nil {
			return total, err
		}
		total += written
	}

	return total, nil
}

func countCovered(slice models.SourceSlice) int {
	covered := 0
	for _, obs := range slice.Hours {
		if hasSourceValues(slice.Kind, obs) {
			covered++
		}
	}

	return covered
}

// columnFor keeps a source from writing columns it does not own.
func columnFor(kind models.SourceKind, value models.Measure) *float64 {
	if !ownsField(kind, value) {
		return nil
	}

	return value.Float64Ptr()
}

func ownsField(kind models.SourceKind, value models.Measure) bool {
	_ = kind

	return value.Known
}

func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}

	return &value
}

func hasSourceValues(kind models.SourceKind, obs models.WeatherObservation) bool {
	for _, factor := range models.FieldsOfKind(kind) {
		if obs.Field(factor).Known {
			return true
		}
	}

	return false
}

// StoredSeries is one place's persisted weather: merged hours plus per-source
// coverage, so callers can see how far each source actually reaches.
type StoredSeries = models.StoredSeries

func (r *WeatherRepository) ListSeries(
	ctx context.Context,
	placeID string,
	from time.Time,
	to time.Time,
) (StoredSeries, error) {
	query := `SELECT ` + observationColumns + `
		FROM weather_observations
		WHERE place_id = $1::uuid
		  AND observed_at >= $2
		  AND observed_at < $3
		ORDER BY observed_at ASC, source ASC`

	rows, err := r.pool.Query(ctx, query, placeID, from, to)
	if err != nil {
		return StoredSeries{}, fmt.Errorf("query weather observations: %w", err)
	}
	defer rows.Close()

	byHour := make(map[time.Time]models.WeatherObservation)
	var geometry models.ModelGeometry
	for rows.Next() {
		obs, sourceRaw, geometryRaw, scanErr := scanObservation(rows)
		if scanErr != nil {
			return StoredSeries{}, scanErr
		}
		kind, kindErr := sourceKindOf(sourceRaw)
		if kindErr != nil {
			return StoredSeries{}, kindErr
		}

		merged, exists := byHour[obs.Hour]
		if !exists {
			merged = models.WeatherObservation{Hour: obs.Hour}
		}
		byHour[obs.Hour] = models.MergeObservations(merged, obs, kind)

		if !geometry.ModelLatitude.Known && len(geometryRaw) > 0 {
			if gErr := json.Unmarshal(geometryRaw, &geometry); gErr != nil {
				return StoredSeries{}, fmt.Errorf("decode model geometry: %w", gErr)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return StoredSeries{}, fmt.Errorf("iterate weather observations: %w", err)
	}

	hours := make([]time.Time, 0, len(byHour))
	for hour := range byHour {
		hours = append(hours, hour)
	}
	sort.Slice(hours, func(i, j int) bool { return hours[i].Before(hours[j]) })

	observations := make([]models.WeatherObservation, 0, len(hours))
	for _, hour := range hours {
		observations = append(observations, byHour[hour])
	}

	statuses, coverage, sErr := r.ListSourceStatus(ctx, placeID)
	if sErr != nil {
		return StoredSeries{}, sErr
	}

	return StoredSeries{
		Observations: observations,
		Sources:      statuses,
		Geometry:     geometry,
		Coverage:     coverage,
	}, nil
}

// ListSourceStatus returns the last known availability and coverage per source.
func (r *WeatherRepository) ListSourceStatus(
	ctx context.Context,
	placeID string,
) ([]models.SourceStatus, []models.SourceCoverage, error) {
	const query = `
		SELECT source, available, reason, demo, covered_from, covered_to, covered_hours, fetched_at
		FROM weather_source_status
		WHERE place_id = $1::uuid
		ORDER BY source ASC
	`

	rows, err := r.pool.Query(ctx, query, placeID)
	if err != nil {
		return nil, nil, fmt.Errorf("query source status: %w", err)
	}
	defer rows.Close()

	statuses := make([]models.SourceStatus, 0, len(models.AllSourceKinds()))
	coverage := make([]models.SourceCoverage, 0, len(models.AllSourceKinds()))
	for rows.Next() {
		var (
			sourceRaw    string
			available    bool
			reason       string
			demo         bool
			coveredFrom  *time.Time
			coveredTo    *time.Time
			coveredHours int
			fetchedAt    time.Time
		)
		if err := rows.Scan(
			&sourceRaw,
			&available,
			&reason,
			&demo,
			&coveredFrom,
			&coveredTo,
			&coveredHours,
			&fetchedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("scan source status: %w", err)
		}

		kind, kindErr := sourceKindOf(sourceRaw)
		if kindErr != nil {
			return nil, nil, kindErr
		}

		statuses = append(statuses, models.SourceStatus{
			Source:    models.SourceID(sourceRaw),
			Available: available,
			Reason:    reason,
			Demo:      demo,
		})

		item := models.SourceCoverage{
			Kind:      kind,
			Available: available,
			Demo:      demo,
			Reason:    reason,
			Hours:     coveredHours,
			Covered:   coveredHours,
			Fields:    models.FieldsOfKind(kind),
			FetchedAt: fetchedAt.UTC().Format(time.RFC3339),
		}
		if coveredFrom != nil {
			item.From = coveredFrom.UTC().Format(time.RFC3339)
		}
		if coveredTo != nil {
			item.To = coveredTo.UTC().Format(time.RFC3339)
		}
		coverage = append(coverage, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate source status: %w", err)
	}

	return statuses, coverage, nil
}

func sourceKindOf(raw string) (models.SourceKind, error) {
	switch models.SourceID(raw) {
	case models.SourceOpenMeteoWeather:
		return models.SourceKindWeather, nil
	case models.SourceOpenMeteoMarine:
		return models.SourceKindMarine, nil
	case models.SourceOpenMeteoAirQuality:
		return models.SourceKindAirQuality, nil
	default:
		return "", fmt.Errorf("unexpected source `%s` in storage", raw)
	}
}

func scanObservation(row pgx.Row) (models.WeatherObservation, string, []byte, error) {
	var (
		placeID         string
		observedAt      time.Time
		sourceRaw       string
		airTemp         *float64
		apparentTemp    *float64
		precipitation   *float64
		windSpeed       *float64
		windGust        *float64
		windDirection   *float64
		humidity        *float64
		cloudCover      *float64
		visibility      *float64
		weatherCode     *float64
		waveHeight      *float64
		wavePeriod      *float64
		waveDirection   *float64
		waterTemp       *float64
		seaLevelHeight  *float64
		airQualityIndex *float64
		pm25            *float64
		geometryRaw     []byte
	)

	if err := row.Scan(
		&placeID,
		&observedAt,
		&sourceRaw,
		&airTemp,
		&apparentTemp,
		&precipitation,
		&windSpeed,
		&windGust,
		&windDirection,
		&humidity,
		&cloudCover,
		&visibility,
		&weatherCode,
		&waveHeight,
		&wavePeriod,
		&waveDirection,
		&waterTemp,
		&seaLevelHeight,
		&airQualityIndex,
		&pm25,
		&geometryRaw,
	); err != nil {
		_ = geometryRaw

		return models.WeatherObservation{}, "", nil, fmt.Errorf("scan weather observation: %w", err)
	}

	return models.WeatherObservation{
		Hour:            observedAt.UTC(),
		AirTempC:        measureFromPtr(airTemp),
		ApparentTempC:   measureFromPtr(apparentTemp),
		PrecipMM:        measureFromPtr(precipitation),
		WindSpeedMS:     measureFromPtr(windSpeed),
		WindGustMS:      measureFromPtr(windGust),
		WindDirection:   measureFromPtr(windDirection),
		HumidityPct:     measureFromPtr(humidity),
		CloudCoverPct:   measureFromPtr(cloudCover),
		VisibilityM:     measureFromPtr(visibility),
		WeatherCode:     measureFromPtr(weatherCode),
		WaveHeightM:     measureFromPtr(waveHeight),
		WavePeriodS:     measureFromPtr(wavePeriod),
		WaveDirection:   measureFromPtr(waveDirection),
		WaterTempC:      measureFromPtr(waterTemp),
		SeaLevelHeightM: measureFromPtr(seaLevelHeight),
		AQIEuropean:     measureFromPtr(airQualityIndex),
		PM25:            measureFromPtr(pm25),
	}, sourceRaw, geometryRaw, nil
}

func measureFromPtr(v *float64) models.Measure {
	if v == nil {
		return models.MissingMeasure()
	}

	return models.NewMeasure(*v)
}

func encodeSources(sources []models.SourceStatus) ([]byte, error) {
	if sources == nil {
		sources = []models.SourceStatus{}
	}
	raw, err := json.Marshal(sources)
	if err != nil {
		return nil, fmt.Errorf("encode source statuses: %w", err)
	}

	return raw, nil
}

func decodeSources(raw []byte) ([]models.SourceStatus, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var sources []models.SourceStatus
	if err := json.Unmarshal(raw, &sources); err != nil {
		return nil, fmt.Errorf("decode source statuses: %w", err)
	}

	return sources, nil
}

func (r *WeatherRepository) DeleteBefore(ctx context.Context, cutoff time.Time) error {
	const query = `DELETE FROM weather_observations WHERE observed_at < $1`
	if _, err := r.pool.Exec(ctx, query, cutoff); err != nil {
		return fmt.Errorf("delete expired weather observations: %w", err)
	}

	return nil
}

func (r *WeatherRepository) execBatch(ctx context.Context, batch *pgx.Batch, expected int) error {
	br := r.pool.SendBatch(ctx, batch)
	for range expected {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()

			return err
		}
	}

	return br.Close()
}

func (r *WeatherRepository) upsertSourceSlice(
	ctx context.Context,
	placeID string,
	slice models.SourceSlice,
) (int, error) {
	if len(slice.Hours) == 0 {
		return 0, r.upsertSourceStatus(ctx, placeID, slice)
	}

	geometryRaw, err := json.Marshal(slice.Geometry)
	if err != nil {
		return 0, fmt.Errorf("encode model geometry: %w", err)
	}

	query := `
		INSERT INTO weather_observations (
			place_id, observed_at, source,
			air_temp_c, apparent_temp_c, precipitation_mm,
			wind_speed_ms, wind_gust_ms, wind_direction_deg,
			relative_humidity_pct, cloud_cover_pct, visibility_m, weather_code,
			wave_height_m, wave_period_s, wave_direction_deg, water_temp_c, sea_level_height_m,
			air_quality_index, pm2_5,
			model_geometry, valid_until, fetched_at
		)
		VALUES (
			$1::uuid, $2, $3,
			$4, $5, $6,
			$7, $8, $9,
			$10, $11, $12, $13,
			$14, $15, $16, $17, $18,
			$19, $20,
			$21::jsonb, $22, $23
		)
		ON CONFLICT (place_id, source, observed_at) DO UPDATE SET
			air_temp_c = EXCLUDED.air_temp_c,
			apparent_temp_c = EXCLUDED.apparent_temp_c,
			precipitation_mm = EXCLUDED.precipitation_mm,
			wind_speed_ms = EXCLUDED.wind_speed_ms,
			wind_gust_ms = EXCLUDED.wind_gust_ms,
			wind_direction_deg = EXCLUDED.wind_direction_deg,
			relative_humidity_pct = EXCLUDED.relative_humidity_pct,
			cloud_cover_pct = EXCLUDED.cloud_cover_pct,
			visibility_m = EXCLUDED.visibility_m,
			weather_code = EXCLUDED.weather_code,
			wave_height_m = EXCLUDED.wave_height_m,
			wave_period_s = EXCLUDED.wave_period_s,
			wave_direction_deg = EXCLUDED.wave_direction_deg,
			water_temp_c = EXCLUDED.water_temp_c,
			sea_level_height_m = EXCLUDED.sea_level_height_m,
			air_quality_index = EXCLUDED.air_quality_index,
			pm2_5 = EXCLUDED.pm2_5,
			model_geometry = EXCLUDED.model_geometry,
			valid_until = EXCLUDED.valid_until,
			fetched_at = EXCLUDED.fetched_at
	`

	observed := make([]models.WeatherObservation, 0, len(slice.Hours))
	for _, obs := range slice.Hours {
		if !hasSourceValues(slice.Kind, obs) {
			continue
		}
		observed = append(observed, obs)
	}
	if len(observed) == 0 {
		return 0, r.upsertSourceStatus(ctx, placeID, slice)
	}

	batch := &pgx.Batch{}
	for _, obs := range observed {
		batch.Queue(
			query,
			placeID,
			obs.Hour.UTC(),
			string(models.SourceIDForKind(slice.Kind)),
			columnFor(slice.Kind, obs.AirTempC),
			columnFor(slice.Kind, obs.ApparentTempC),
			columnFor(slice.Kind, obs.PrecipMM),
			columnFor(slice.Kind, obs.WindSpeedMS),
			columnFor(slice.Kind, obs.WindGustMS),
			columnFor(slice.Kind, obs.WindDirection),
			columnFor(slice.Kind, obs.HumidityPct),
			columnFor(slice.Kind, obs.CloudCoverPct),
			columnFor(slice.Kind, obs.VisibilityM),
			columnFor(slice.Kind, obs.WeatherCode),
			columnFor(slice.Kind, obs.WaveHeightM),
			columnFor(slice.Kind, obs.WavePeriodS),
			columnFor(slice.Kind, obs.WaveDirection),
			columnFor(slice.Kind, obs.WaterTempC),
			columnFor(slice.Kind, obs.SeaLevelHeightM),
			columnFor(slice.Kind, obs.AQIEuropean),
			columnFor(slice.Kind, obs.PM25),
			string(geometryRaw),
			nullableTime(slice.ValidUntil),
			slice.FetchedAt.UTC(),
		)
	}

	if err := r.execBatch(ctx, batch, len(observed)); err != nil {
		return 0, fmt.Errorf("upsert %s observations for place `%s`: %w", slice.Kind, placeID, err)
	}
	if err := r.upsertSourceStatus(ctx, placeID, slice); err != nil {
		return 0, err
	}

	return len(observed), nil
}

func (r *WeatherRepository) upsertSourceStatus(
	ctx context.Context,
	placeID string,
	slice models.SourceSlice,
) error {
	const query = `
		INSERT INTO weather_source_status (
			place_id, source, available, reason, demo,
			covered_from, covered_to, covered_hours, fetched_at
		)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (place_id, source) DO UPDATE SET
			available = EXCLUDED.available,
			reason = EXCLUDED.reason,
			demo = EXCLUDED.demo,
			covered_from = EXCLUDED.covered_from,
			covered_to = EXCLUDED.covered_to,
			covered_hours = EXCLUDED.covered_hours,
			fetched_at = EXCLUDED.fetched_at
	`

	// Coverage must describe the hours that actually carry values, not the
	// whole requested range: AQI legitimately ends earlier than weather.
	var (
		from time.Time
		to   time.Time
	)
	for _, obs := range slice.Hours {
		if !hasSourceValues(slice.Kind, obs) {
			continue
		}
		if from.IsZero() {
			from = obs.Hour
		}
		to = obs.Hour.Add(time.Hour)
	}
	covered := countCovered(slice)

	if _, err := r.pool.Exec(
		ctx,
		query,
		placeID,
		string(models.SourceIDForKind(slice.Kind)),
		slice.Status.Available,
		slice.Status.Reason,
		slice.Status.Demo,
		nullableTime(from),
		nullableTime(to),
		covered,
		slice.FetchedAt.UTC(),
	); err != nil {
		return fmt.Errorf("upsert %s status for place `%s`: %w", slice.Kind, placeID, err)
	}

	return nil
}
