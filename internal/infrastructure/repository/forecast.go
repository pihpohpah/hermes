package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

type ForecastRepository struct {
	pool *pgxpool.Pool
}

func NewForecastRepository(pool *pgxpool.Pool) *ForecastRepository {
	return &ForecastRepository{pool: pool}
}

func (r *ForecastRepository) UpsertBatch(ctx context.Context, assessments []models.ConditionAssessment) error {
	if len(assessments) == 0 {
		return nil
	}

	const query = `
		INSERT INTO conditions_cache (
			place_id, activity, forecast_time,
			score, verdict, confidence, data, factors,
			contributions, warnings, limitations, summary,
			sources, demo, algorithm_version, computed_at
		)
		VALUES (
			$1::uuid, $2, $3,
			$4, $5, $6, $7::jsonb, $8::jsonb,
			$9::jsonb, $10::jsonb, $11::jsonb, $12,
			$13::jsonb, $14, $15, $16
		)
		ON CONFLICT (place_id, activity, forecast_time) DO UPDATE SET
			score = EXCLUDED.score,
			verdict = EXCLUDED.verdict,
			confidence = EXCLUDED.confidence,
			data = EXCLUDED.data,
			factors = EXCLUDED.factors,
			contributions = EXCLUDED.contributions,
			warnings = EXCLUDED.warnings,
			limitations = EXCLUDED.limitations,
			summary = EXCLUDED.summary,
			sources = EXCLUDED.sources,
			demo = EXCLUDED.demo,
			algorithm_version = EXCLUDED.algorithm_version,
			computed_at = EXCLUDED.computed_at
	`

	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, item := range assessments {
		encoded, err := encodeAssessment(item, now)
		if err != nil {
			return err
		}
		batch.Queue(
			query,
			item.PlaceID,
			string(item.Activity),
			item.ForecastTime.UTC(),
			int(item.Score),
			string(item.Verdict),
			string(item.Confidence),
			string(encoded.rawData),
			string(encoded.factors),
			string(encoded.contributions),
			string(encoded.warnings),
			string(encoded.limitations),
			item.Summary,
			string(encoded.sources),
			item.Demo,
			item.AlgorithmVersion,
			now,
		)
	}

	br := r.pool.SendBatch(ctx, batch)
	for _, item := range assessments {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()

			return fmt.Errorf("upsert assessment for place `%s`/`%s`: %w", item.PlaceID, item.Activity, err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("close assessment upsert batch: %w", err)
	}

	return nil
}

const assessmentColumns = `
	place_id::text,
	activity,
	forecast_time,
	score,
	verdict,
	confidence,
	data,
	factors,
	contributions,
	warnings,
	limitations,
	summary,
	sources,
	demo,
	algorithm_version
`

func (r *ForecastRepository) ListByPlaceAndRange(
	ctx context.Context,
	placeID string,
	activity models.Activity,
	from time.Time,
	to time.Time,
) ([]models.ConditionAssessment, error) {
	query := `SELECT ` + assessmentColumns + `
		FROM conditions_cache
		WHERE place_id = $1::uuid
		  AND activity = $2
		  AND forecast_time >= $3
		  AND forecast_time < $4
		ORDER BY forecast_time ASC, activity ASC`

	rows, err := r.pool.Query(ctx, query, placeID, string(activity), from, to)
	if err != nil {
		return nil, fmt.Errorf("query conditions cache: %w", err)
	}
	defer rows.Close()

	result := make([]models.ConditionAssessment, 0)
	for rows.Next() {
		assessment, scanErr := scanAssessment(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, assessment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conditions cache rows: %w", err)
	}

	return result, nil
}

func (r *ForecastRepository) DeleteBefore(ctx context.Context, cutoff time.Time) error {
	const query = `DELETE FROM conditions_cache WHERE forecast_time < $1`
	if _, err := r.pool.Exec(ctx, query, cutoff); err != nil {
		return fmt.Errorf("delete expired conditions cache: %w", err)
	}

	return nil
}

type storedAssessment struct {
	rawData       []byte
	factors       []byte
	contributions []byte
	warnings      []byte
	limitations   []byte
	sources       []byte
}

type factorStorageDTO struct {
	AirTempC        json.RawMessage `json:"air_temp_c"`
	ApparentTempC   json.RawMessage `json:"apparent_temp_c"`
	PrecipitationMM json.RawMessage `json:"precipitation_mm"`
	WindSpeedMS     json.RawMessage `json:"wind_speed_ms"`
	WindGustMS      json.RawMessage `json:"wind_gust_ms"`
	WaveHeightM     json.RawMessage `json:"wave_height_m"`
	WaterTempC      json.RawMessage `json:"water_temp_c"`
	AirQualityIndex json.RawMessage `json:"air_quality_index"`
	VisibilityM     json.RawMessage `json:"visibility_m"`
	WeatherCode     json.RawMessage `json:"weather_code"`
}

func encodeAssessment(item models.ConditionAssessment, computedAt time.Time) (storedAssessment, error) {
	rawData, err := json.Marshal(map[string]any{
		"activity":          item.Activity,
		"local_date":        item.LocalDate,
		"algorithm_version": item.AlgorithmVersion,
		"computed_at":       computedAt.Format(time.RFC3339),
		"geometry":          item.Geometry,
		"coverage":          item.Coverage,
	})
	if err != nil {
		return storedAssessment{}, fmt.Errorf("encode assessment payload: %w", err)
	}

	factors, err := json.Marshal(factorStorageDTO{
		AirTempC:        rawMeasure(item.Observation.AirTempC),
		ApparentTempC:   rawMeasure(item.Observation.ApparentTempC),
		PrecipitationMM: rawMeasure(item.Observation.PrecipMM),
		WindSpeedMS:     rawMeasure(item.Observation.WindSpeedMS),
		WindGustMS:      rawMeasure(item.Observation.WindGustMS),
		WaveHeightM:     rawMeasure(item.Observation.WaveHeightM),
		WaterTempC:      rawMeasure(item.Observation.WaterTempC),
		AirQualityIndex: rawMeasure(item.Observation.AQIEuropean),
		VisibilityM:     rawMeasure(item.Observation.VisibilityM),
		WeatherCode:     rawMeasure(item.Observation.WeatherCode),
	})
	if err != nil {
		return storedAssessment{}, fmt.Errorf("encode assessment factors: %w", err)
	}

	contributions, err := json.Marshal(nonNil(item.Contributions))
	if err != nil {
		return storedAssessment{}, fmt.Errorf("encode assessment contributions: %w", err)
	}
	warnings, err := json.Marshal(nonNil(item.Warnings))
	if err != nil {
		return storedAssessment{}, fmt.Errorf("encode assessment warnings: %w", err)
	}
	limitations, err := json.Marshal(nonNil(item.Limitations))
	if err != nil {
		return storedAssessment{}, fmt.Errorf("encode assessment limitations: %w", err)
	}
	sources, err := encodeSources(item.Sources)
	if err != nil {
		return storedAssessment{}, err
	}

	return storedAssessment{
		rawData:       rawData,
		factors:       factors,
		contributions: contributions,
		warnings:      warnings,
		limitations:   limitations,
		sources:       sources,
	}, nil
}

func rawMeasure(m models.Measure) json.RawMessage {
	if !m.Known {
		return json.RawMessage("null")
	}

	return json.RawMessage(fmt.Sprintf("%v", m.Value))
}

func scanAssessment(row pgx.Row) (models.ConditionAssessment, error) {
	var (
		placeID          string
		activityRaw      string
		forecastTime     time.Time
		scoreRaw         int
		verdictRaw       string
		confidenceRaw    string
		rawData          []byte
		factorsRaw       []byte
		contributionsRaw []byte
		warningsRaw      []byte
		limitationsRaw   []byte
		summary          string
		sourcesRaw       []byte
		demo             bool
		algorithm        string
	)

	if err := row.Scan(
		&placeID,
		&activityRaw,
		&forecastTime,
		&scoreRaw,
		&verdictRaw,
		&confidenceRaw,
		&rawData,
		&factorsRaw,
		&contributionsRaw,
		&warningsRaw,
		&limitationsRaw,
		&summary,
		&sourcesRaw,
		&demo,
		&algorithm,
	); err != nil {
		return models.ConditionAssessment{}, fmt.Errorf("scan conditions cache row: %w", err)
	}

	activity, err := models.NewActivity(activityRaw)
	if err != nil {
		return models.ConditionAssessment{}, fmt.Errorf("parse activity from db `%s`: %w", activityRaw, err)
	}
	score, err := models.NewScore(scoreRaw)
	if err != nil {
		return models.ConditionAssessment{}, fmt.Errorf("parse score from db: %w", err)
	}
	verdict, err := models.NewVerdict(verdictRaw)
	if err != nil {
		return models.ConditionAssessment{}, fmt.Errorf("parse verdict from db: %w", err)
	}
	confidence, err := models.NewConfidence(confidenceRaw)
	if err != nil {
		return models.ConditionAssessment{}, fmt.Errorf("parse confidence from db: %w", err)
	}

	factors, err := decodeFactors(factorsRaw)
	if err != nil {
		return models.ConditionAssessment{}, err
	}
	var contributions []models.Contribution
	if len(contributionsRaw) > 0 {
		if uErr := json.Unmarshal(contributionsRaw, &contributions); uErr != nil {
			return models.ConditionAssessment{}, fmt.Errorf("decode assessment contributions: %w", uErr)
		}
	}
	var warnings []models.Warning
	if len(warningsRaw) > 0 {
		if uErr := json.Unmarshal(warningsRaw, &warnings); uErr != nil {
			return models.ConditionAssessment{}, fmt.Errorf("decode assessment warnings: %w", uErr)
		}
	}
	var limitations []string
	if len(limitationsRaw) > 0 {
		if uErr := json.Unmarshal(limitationsRaw, &limitations); uErr != nil {
			return models.ConditionAssessment{}, fmt.Errorf("decode assessment limitations: %w", uErr)
		}
	}
	sources, err := decodeSources(sourcesRaw)
	if err != nil {
		return models.ConditionAssessment{}, err
	}

	var payload struct {
		LocalDate string                  `json:"local_date"`
		Geometry  models.ModelGeometry    `json:"geometry"`
		Coverage  []models.SourceCoverage `json:"coverage"`
	}
	if len(rawData) > 0 {
		if uErr := json.Unmarshal(rawData, &payload); uErr != nil {
			return models.ConditionAssessment{}, fmt.Errorf("decode assessment payload: %w", uErr)
		}
	}

	return models.ConditionAssessment{
		PlaceID:          placeID,
		Activity:         activity,
		ForecastTime:     forecastTime.UTC(),
		LocalDate:        payload.LocalDate,
		Observation:      decodeObservation(factors),
		Geometry:         payload.Geometry,
		Coverage:         payload.Coverage,
		Score:            score,
		Verdict:          verdict,
		Confidence:       confidence,
		Limitations:      limitations,
		Summary:          summary,
		Contributions:    contributions,
		Warnings:         warnings,
		Sources:          sources,
		Demo:             demo,
		AlgorithmVersion: algorithm,
	}, nil
}

func decodeObservation(dto factorStorageDTO) models.WeatherObservation {
	return models.WeatherObservation{
		AirTempC:      decodeMeasure(dto.AirTempC),
		ApparentTempC: decodeMeasure(dto.ApparentTempC),
		PrecipMM:      decodeMeasure(dto.PrecipitationMM),
		WindSpeedMS:   decodeMeasure(dto.WindSpeedMS),
		WindGustMS:    decodeMeasure(dto.WindGustMS),
		WaveHeightM:   decodeMeasure(dto.WaveHeightM),
		WaterTempC:    decodeMeasure(dto.WaterTempC),
		AQIEuropean:   decodeMeasure(dto.AirQualityIndex),
		VisibilityM:   decodeMeasure(dto.VisibilityM),
		WeatherCode:   decodeMeasure(dto.WeatherCode),
	}
}

func decodeMeasure(raw json.RawMessage) models.Measure {
	var m models.Measure
	if len(raw) == 0 {
		return m
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return models.MissingMeasure()
	}

	return m
}

func decodeFactors(raw []byte) (factorStorageDTO, error) {
	var dto factorStorageDTO
	if len(raw) == 0 {
		return dto, nil
	}
	if err := json.Unmarshal(raw, &dto); err != nil {
		return factorStorageDTO{}, fmt.Errorf("decode condition factors: %w", err)
	}

	return dto, nil
}

func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}

	return values
}
