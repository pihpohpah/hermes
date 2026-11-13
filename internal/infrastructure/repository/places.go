package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/domain/models"
	"github.com/pihpohpah/hermes/internal/domain/scoring"
)

type PlaceRepository struct {
	pool *pgxpool.Pool
}

func NewPlaceRepository(pool *pgxpool.Pool) *PlaceRepository {
	return &PlaceRepository{pool: pool}
}

const (
	defaultPlaceListLimit = 100

	geoLatOffset    = 1
	geoRadiusOffset = 2
	geoArgCount     = 3
)

func (r *PlaceRepository) List(ctx context.Context, filter models.PlaceFilter) ([]models.Place, error) {
	query, args := buildPlaceListQuery(filter)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query places: %w", err)
	}
	defer rows.Close()

	result := make([]models.Place, 0)
	for rows.Next() {
		place, scanErr := scanPlace(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, place)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate places rows: %w", err)
	}

	return result, nil
}

func (r *PlaceRepository) GetByID(ctx context.Context, id string) (models.Place, error) {
	const query = `
		SELECT
			p.id::text,
			p.name,
			p.description,
			p.type,
			p.layer_code,
			ST_Y(p.location::geometry) AS lat,
			ST_X(p.location::geometry) AS lon,
			NULL::double precision AS distance_m,
			p.elevation,
			p.elevation_source,
			p.difficulty,
			p.osm_id,
			p.created_at,
			c.forecast_time,
			c.activity,
			c.score,
			c.verdict,
			c.confidence,
			c.factors,
			c.contributions,
			c.warnings,
			c.summary
		FROM places p
		LEFT JOIN LATERAL (
			SELECT
				forecast_time, activity, score, verdict, confidence,
				factors, contributions, warnings, summary
			FROM conditions_cache cc
			WHERE cc.place_id = p.id
			ORDER BY
				CASE WHEN cc.activity = $1 THEN 0 ELSE 1 END,
				CASE WHEN cc.algorithm_version = $2 THEN 0 ELSE 1 END,
				cc.forecast_time DESC
			LIMIT 1
		) c ON true
		WHERE p.id = $3::uuid
	`

	place, err := scanPlace(
		r.pool.QueryRow(ctx, query, string(models.ActivityWalk), scoring.AlgorithmVersion, id),
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Place{}, models.ErrNotFound
		}

		return models.Place{}, fmt.Errorf("query place by id: %w", err)
	}

	return place, nil
}

func (r *PlaceRepository) Create(ctx context.Context, in models.CreatePlaceInput) (models.Place, error) {
	const query = `
		INSERT INTO places (
			name,
			description,
			type,
			layer_code,
			location,
			elevation,
			elevation_source,
			difficulty,
			osm_id
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			ST_SetSRID(ST_MakePoint($5, $6), 4326)::geography,
			$7,
			$8,
			$9,
			$10
		)
		ON CONFLICT (osm_id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			type = EXCLUDED.type,
			layer_code = EXCLUDED.layer_code,
			location = EXCLUDED.location,
			elevation = COALESCE(places.elevation, EXCLUDED.elevation),
			elevation_source = COALESCE(places.elevation_source, EXCLUDED.elevation_source),
			difficulty = EXCLUDED.difficulty
		RETURNING
			id::text,
			name,
			description,
			type,
			layer_code,
			ST_Y(location::geometry) AS lat,
			ST_X(location::geometry) AS lon,
			NULL::double precision AS distance_m,
			elevation,
			elevation_source,
			difficulty,
			osm_id,
			created_at,
			NULL::timestamptz AS forecast_time,
			NULL::text AS activity,
			NULL::int AS score,
			NULL::text AS verdict,
			NULL::text AS confidence,
			NULL::jsonb AS factors,
			NULL::jsonb AS contributions,
			NULL::jsonb AS warnings,
			NULL::text AS summary
	`

	var diffPtr *int
	if in.Difficulty != nil {
		d := int(*in.Difficulty)
		diffPtr = &d
	}

	row := r.pool.QueryRow(
		ctx,
		query,
		in.Name,
		in.Description,
		string(in.Type),
		string(in.Layer),
		in.Location.Lon,
		in.Location.Lat,
		in.Elevation,
		elevationSourceValue(in),
		diffPtr,
		in.OSMID,
	)

	place, err := scanPlace(row)
	if err != nil {
		return models.Place{}, fmt.Errorf("insert place: %w", err)
	}

	return place, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPlace(row rowScanner) (models.Place, error) {
	var (
		id               string
		name             string
		description      string
		typeRaw          string
		layerRaw         string
		lat              float64
		lon              float64
		distanceM        *float64
		elevation        *int
		elevationSource  *string
		difficulty       *int
		osmID            *int64
		createdAt        time.Time
		forecastTime     *time.Time
		activityRaw      *string
		scoreRaw         *int
		verdictRaw       *string
		confidenceRaw    *string
		factorsRaw       []byte
		contributionsRaw []byte
		warningsRaw      []byte
		summary          *string
	)

	if err := row.Scan(
		&id,
		&name,
		&description,
		&typeRaw,
		&layerRaw,
		&lat,
		&lon,
		&distanceM,
		&elevation,
		&elevationSource,
		&difficulty,
		&osmID,
		&createdAt,
		&forecastTime,
		&activityRaw,
		&scoreRaw,
		&verdictRaw,
		&confidenceRaw,
		&factorsRaw,
		&contributionsRaw,
		&warningsRaw,
		&summary,
	); err != nil {
		return models.Place{}, fmt.Errorf("scan place row: %w", err)
	}

	pt, err := models.NewPlaceType(typeRaw)
	if err != nil {
		return models.Place{}, fmt.Errorf("parse place type from db `%s`: %w", typeRaw, err)
	}
	layer, err := models.NewLayerCode(layerRaw)
	if err != nil {
		return models.Place{}, fmt.Errorf("parse layer code from db `%s`: %w", layerRaw, err)
	}
	loc, err := models.NewPoint(lat, lon)
	if err != nil {
		return models.Place{}, fmt.Errorf("parse place location: %w", err)
	}

	var diffPtr *models.Difficulty
	if difficulty != nil {
		d, dErr := models.NewDifficulty(*difficulty)
		if dErr != nil {
			return models.Place{}, fmt.Errorf("parse difficulty from db: %w", dErr)
		}
		diffPtr = &d
	}

	latest, err := scanLatestAssessment(id, latestAssessmentRow{
		forecastTime:  forecastTime,
		activity:      activityRaw,
		score:         scoreRaw,
		verdict:       verdictRaw,
		confidence:    confidenceRaw,
		factors:       factorsRaw,
		contributions: contributionsRaw,
		warnings:      warningsRaw,
		summary:       summary,
	})
	if err != nil {
		return models.Place{}, err
	}

	return models.Place{
		ID:              id,
		Name:            name,
		Description:     description,
		Type:            pt,
		Layer:           layer,
		Location:        loc,
		DistanceM:       distanceM,
		Elevation:       elevation,
		ElevationSource: elevationSourceOf(elevationSource),
		Difficulty:      diffPtr,
		OSMID:           osmID,
		LatestScore:     latest,
		CreatedAt:       createdAt,
	}, nil
}

type latestAssessmentRow struct {
	forecastTime  *time.Time
	activity      *string
	score         *int
	verdict       *string
	confidence    *string
	factors       []byte
	contributions []byte
	warnings      []byte
	summary       *string
}

func scanLatestAssessment(placeID string, row latestAssessmentRow) (*models.ConditionAssessment, error) {
	if row.forecastTime == nil || row.score == nil {
		return nil, nil
	}

	score, err := models.NewScore(*row.score)
	if err != nil {
		return nil, fmt.Errorf("parse latest score from db: %w", err)
	}

	assessment := &models.ConditionAssessment{
		PlaceID:      placeID,
		Activity:     models.ActivityWalk,
		ForecastTime: row.forecastTime.UTC(),
		Score:        score,
		Verdict:      models.VerdictCaution,
		Confidence:   models.ConfidenceLow,
	}
	if row.activity != nil {
		activity, aErr := models.NewActivity(*row.activity)
		if aErr != nil {
			return nil, fmt.Errorf("parse latest activity from db: %w", aErr)
		}
		assessment.Activity = activity
	}
	if row.verdict != nil {
		verdict, vErr := models.NewVerdict(*row.verdict)
		if vErr != nil {
			return nil, fmt.Errorf("parse latest verdict from db: %w", vErr)
		}
		assessment.Verdict = verdict
	}
	if row.confidence != nil {
		confidence, cErr := models.NewConfidence(*row.confidence)
		if cErr != nil {
			return nil, fmt.Errorf("parse latest confidence from db: %w", cErr)
		}
		assessment.Confidence = confidence
	}

	factors, fErr := decodeFactors(row.factors)
	if fErr != nil {
		return nil, fErr
	}
	assessment.Observation = decodeObservation(factors)
	if row.summary != nil {
		assessment.Summary = *row.summary
	}
	if err := decodeJSONInto(row.contributions, &assessment.Contributions); err != nil {
		return nil, fmt.Errorf("decode latest contributions: %w", err)
	}
	if err := decodeJSONInto(row.warnings, &assessment.Warnings); err != nil {
		return nil, fmt.Errorf("decode latest warnings: %w", err)
	}

	return assessment, nil
}

func decodeJSONInto[T any](raw []byte, dst *T) error {
	if len(raw) == 0 {
		return nil
	}

	return json.Unmarshal(raw, dst)
}

// listLeadingArgs are bound by the latest-assessment lateral join.
func listLeadingArgs() []any {
	return []any{string(models.ActivityWalk), scoring.AlgorithmVersion}
}

// buildPlaceListQuery builds the filtered place list SQL together with its
// arguments. Placeholders are numbered dynamically, so the number of leading
// lateral-join arguments is added to every offset exactly once.
func buildPlaceListQuery(filter models.PlaceFilter) (string, []any) {
	leading := listLeadingArgs()
	argPos := len(leading) + 1

	var (
		where      []string
		args       = make([]any, 0, argPos)
		distSelect = "NULL::double precision AS distance_m"
		orderBy    = "p.created_at DESC"
	)

	if filter.Center != nil && filter.RadiusM != nil {
		lonPos := argPos
		latPos := lonPos + geoLatOffset
		radPos := lonPos + geoRadiusOffset
		argPos += geoArgCount
		args = append(args, filter.Center.Lon, filter.Center.Lat, float64(*filter.RadiusM))

		where = append(
			where,
			fmt.Sprintf("ST_DWithin(p.location, ST_MakePoint($%d,$%d)::geography, $%d)", lonPos, latPos, radPos),
		)
		distSelect = fmt.Sprintf(
			"ST_Distance(p.location, ST_MakePoint($%d,$%d)::geography) AS distance_m",
			lonPos,
			latPos,
		)
		orderBy = "distance_m ASC"
	}

	if filter.Type != nil {
		where = append(where, fmt.Sprintf("p.type = $%d", argPos))
		args = append(args, string(*filter.Type))
		argPos++
	}

	if filter.Layer != nil {
		where = append(where, fmt.Sprintf("p.layer_code = $%d", argPos))
		args = append(args, string(*filter.Layer))
		argPos++
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultPlaceListLimit
	}
	args = append(args, limit)
	limitPos := argPos

	query := fmt.Sprintf(`
		SELECT
			p.id::text,
			p.name,
			p.description,
			p.type,
			p.layer_code,
			ST_Y(p.location::geometry) AS lat,
			ST_X(p.location::geometry) AS lon,
			%s,
			p.elevation,
			p.elevation_source,
			p.difficulty,
			p.osm_id,
			p.created_at,
			c.forecast_time,
			c.activity,
			c.score,
			c.verdict,
			c.confidence,
			c.factors,
			c.contributions,
			c.warnings,
			c.summary
		FROM places p
		LEFT JOIN LATERAL (
			SELECT
				forecast_time, activity, score, verdict, confidence,
				factors, contributions, warnings, summary
			FROM conditions_cache cc
			WHERE cc.place_id = p.id
			ORDER BY
				CASE WHEN cc.activity = $1 THEN 0 ELSE 1 END,
				CASE WHEN cc.algorithm_version = $2 THEN 0 ELSE 1 END,
				cc.forecast_time DESC
			LIMIT 1
		) c ON true
	`, distSelect)

	if len(where) > 0 {
		query = fmt.Sprintf("%s WHERE %s", query, strings.Join(where, " AND "))
	}
	query = fmt.Sprintf("%s ORDER BY %s LIMIT $%d", query, orderBy, limitPos)

	return query, append(leading, args...)
}

// SetElevation stores a resolved altitude together with its provenance.
func (r *PlaceRepository) SetElevation(
	ctx context.Context,
	placeID string,
	meters int,
	source models.ElevationSource,
) error {
	const query = `
		UPDATE places
		SET elevation = $2,
		    elevation_source = $3
		WHERE id = $1::uuid
	`

	tag, err := r.pool.Exec(ctx, query, placeID, meters, string(source))
	if err != nil {
		return fmt.Errorf("update place elevation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrNotFound
	}

	return nil
}

func elevationSourceValue(in models.CreatePlaceInput) *string {
	if in.ElevationSource == models.ElevationSourceUnknown {
		return nil
	}
	source := string(in.ElevationSource)

	return &source
}

func elevationSourceOf(raw *string) models.ElevationSource {
	if raw == nil {
		return models.ElevationSourceUnknown
	}

	source, err := models.NewElevationSource(*raw)
	if err != nil {
		return models.ElevationSourceUnknown
	}

	return source
}
