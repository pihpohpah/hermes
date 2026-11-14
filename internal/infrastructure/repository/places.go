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

// placeColumns is the shared SELECT list of the place row with its latest
// assessment lateral join. %s is the distance expression.
const placeColumns = `
	p.id::text,
	p.slug,
	p.name,
	p.subtitle,
	p.short,
	p.description,
	p.type,
	p.layer_code,
	p.category,
	p.district,
	p.address,
	ST_Y(p.location::geometry) AS lat,
	ST_X(p.location::geometry) AS lon,
	%s,
	p.hours,
	p.price_free,
	p.price_label,
	p.duration,
	p.season,
	p.access,
	p.how_to_get,
	p.facts,
	p.tags,
	p.photos,
	p.beach,
	p.swim_season,
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
`

// latestAssessmentJoin is bound by the two leading lateral-join arguments:
// preferred activity and preferred algorithm version.
const latestAssessmentJoin = `
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
`

// listLeadingArgs are bound by the latest-assessment lateral join.
func listLeadingArgs() []any {
	return []any{string(models.ActivityWalk), scoring.AlgorithmVersion}
}

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

// GetByID resolves a place by UUID or by its unique slug, so both the API
// identifiers and the human-readable URL part work.
func (r *PlaceRepository) GetByID(ctx context.Context, id string) (models.Place, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM places p
		%s
		WHERE p.id::text = $3 OR p.slug = $3
	`, fmt.Sprintf(placeColumns, "NULL::double precision AS distance_m"), latestAssessmentJoin)

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

// ListSimilar returns places of the same scenario, preferring the same
// category, then the same type, then closest by straight-line distance.
// Placeholders $1/$2 belong to the lateral join, the place key is $3.
func (r *PlaceRepository) ListSimilar(ctx context.Context, id string, limit int) ([]models.Place, error) {
	origin, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	const rawQuery = `
		WITH origin AS (
			SELECT id, location, layer_code, category, type
			FROM places
			WHERE id::text = $3 OR slug = $3
		)
		SELECT ` + placeColumns + `
		FROM places p
		CROSS JOIN origin o
	` + latestAssessmentJoin + `
		WHERE p.id <> o.id
			AND p.layer_code = o.layer_code
			AND (
				(o.category IS NOT NULL AND p.category = o.category)
				OR p.type = o.type
			)
		ORDER BY
			CASE
				WHEN o.category IS NOT NULL AND p.category = o.category THEN 0
				WHEN p.type = o.type THEN 1
				ELSE 2
			END,
			ST_Distance(p.location, o.location) ASC
		LIMIT $4
	`

	query := fmt.Sprintf(rawQuery, "NULL::double precision AS distance_m")

	rows, err := r.pool.Query(
		ctx,
		query,
		string(models.ActivityWalk),
		scoring.AlgorithmVersion,
		origin.ID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query similar places: %w", err)
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
		return nil, fmt.Errorf("iterate similar places rows: %w", err)
	}

	return result, nil
}

// Stats counts catalog entries for one scenario layer (nil = all layers).
func (r *PlaceRepository) Stats(ctx context.Context, layer *models.LayerCode) (models.PlaceStats, error) {
	stats := models.PlaceStats{
		CategoryCounts:      make(map[string]int),
		EventCategoryCounts: make(map[string]int),
	}

	placeQuery := `SELECT category, count(*) FROM places`
	eventQuery := `
		SELECT e.category, count(*)
		FROM events e
		LEFT JOIN places p ON p.id = e.place_id
		WHERE e.ends_at > now()
	`
	args := make([]any, 0, 1)

	if layer != nil {
		args = append(args, string(*layer))
		placeQuery += fmt.Sprintf(" WHERE layer_code = $%d", len(args))
		eventQuery += fmt.Sprintf(" AND p.layer_code = $%d", len(args))
	}

	placeQuery += " GROUP BY category"
	eventQuery += " GROUP BY e.category"

	placeRows, err := r.pool.Query(ctx, placeQuery, args...)
	if err != nil {
		return models.PlaceStats{}, fmt.Errorf("query place stats: %w", err)
	}
	for placeRows.Next() {
		var category *string
		var count int
		if err := placeRows.Scan(&category, &count); err != nil {
			placeRows.Close()

			return models.PlaceStats{}, fmt.Errorf("scan place stats: %w", err)
		}
		if category != nil {
			stats.CategoryCounts[*category] = count
			stats.Places += count
		}
	}
	placeRows.Close()
	if err := placeRows.Err(); err != nil {
		return models.PlaceStats{}, fmt.Errorf("iterate place stats: %w", err)
	}

	eventRows, err := r.pool.Query(ctx, eventQuery, args...)
	if err != nil {
		return models.PlaceStats{}, fmt.Errorf("query event stats: %w", err)
	}
	for eventRows.Next() {
		var category string
		var count int
		if err := eventRows.Scan(&category, &count); err != nil {
			eventRows.Close()

			return models.PlaceStats{}, fmt.Errorf("scan event stats: %w", err)
		}
		stats.EventCategoryCounts[category] = count
		stats.Events += count
	}
	eventRows.Close()
	if err := eventRows.Err(); err != nil {
		return models.PlaceStats{}, fmt.Errorf("iterate event stats: %w", err)
	}

	return stats, nil
}

// GetManyByIDs loads a set of places for collection hydration. Unknown ids
// are silently skipped.
func (r *PlaceRepository) GetManyByIDs(ctx context.Context, ids []string) ([]models.Place, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM places p
		%s
		WHERE p.id::text = ANY($3::text[])
	`, fmt.Sprintf(placeColumns, "NULL::double precision AS distance_m"), latestAssessmentJoin)

	rows, err := r.pool.Query(ctx, query, string(models.ActivityWalk), scoring.AlgorithmVersion, ids)
	if err != nil {
		return nil, fmt.Errorf("query places by ids: %w", err)
	}
	defer rows.Close()

	result := make([]models.Place, 0, len(ids))
	for rows.Next() {
		place, scanErr := scanPlace(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, place)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate places by ids rows: %w", err)
	}

	return result, nil
}

func (r *PlaceRepository) Create(ctx context.Context, in models.CreatePlaceInput) (models.Place, error) {
	const query = `
		INSERT INTO places (
			slug, name, subtitle, short, description,
			type, layer_code, category, district, address, location,
			hours, price_free, price_label, duration, season,
			access, how_to_get, facts, tags, photos, beach, swim_season,
			elevation, elevation_source, difficulty, osm_id
		)
		VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10, ST_SetSRID(ST_MakePoint($11, $12), 4326)::geography,
			$13, $14, $15, $16, $17,
			$18, $19, $20, $21, $22, $23, $24,
			$25, $26, $27, $28
		)
		ON CONFLICT (osm_id) DO UPDATE SET
			name = EXCLUDED.name,
			subtitle = EXCLUDED.subtitle,
			short = EXCLUDED.short,
			description = EXCLUDED.description,
			type = EXCLUDED.type,
			layer_code = EXCLUDED.layer_code,
			category = EXCLUDED.category,
			district = EXCLUDED.district,
			address = EXCLUDED.address,
			location = EXCLUDED.location,
			hours = EXCLUDED.hours,
			price_free = EXCLUDED.price_free,
			price_label = EXCLUDED.price_label,
			duration = EXCLUDED.duration,
			season = EXCLUDED.season,
			access = EXCLUDED.access,
			how_to_get = EXCLUDED.how_to_get,
			facts = EXCLUDED.facts,
			tags = EXCLUDED.tags,
			photos = EXCLUDED.photos,
			beach = EXCLUDED.beach,
			swim_season = EXCLUDED.swim_season,
			elevation = COALESCE(places.elevation, EXCLUDED.elevation),
			elevation_source = COALESCE(places.elevation_source, EXCLUDED.elevation_source),
			difficulty = EXCLUDED.difficulty
		RETURNING id`

	accessModes := make([]string, 0, len(in.Access))
	for _, mode := range in.Access {
		accessModes = append(accessModes, string(mode))
	}

	args := []any{
		in.SlugOrDefault(),
		in.Name,
		in.Subtitle,
		in.Short,
		in.Description,
		string(in.Type),
		string(in.Layer),
		categoryValue(in.Category),
		in.District,
		in.Address,
		in.Location.Lon,
		in.Location.Lat,
		in.Hours,
		in.PriceFree,
		in.PriceLabel,
		in.Duration,
		in.Season,
		accessModes,
		in.HowToGet,
		append([]string{}, in.Facts...),
		append([]string{}, in.Tags...),
		photosValue(in.Photos),
		beachValue(in.Beach),
		in.SwimSeason,
		in.Elevation,
		elevationSourceValue(in),
		difficultyValue(in.Difficulty),
		in.OSMID,
	}

	// placeColumns нельзя вернуть из RETURNING напрямую: в нём алиасы и JOIN
	// с оценкой условий. Поэтому вставляем и тут же читаем запись целиком.
	var createdID string
	if err := r.pool.QueryRow(ctx, query, args...).Scan(&createdID); err != nil {
		return models.Place{}, fmt.Errorf("insert place: %w", err)
	}

	place, err := r.GetByID(ctx, createdID)
	if err != nil {
		return models.Place{}, fmt.Errorf("load created place: %w", err)
	}

	return place, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPlace(row rowScanner) (models.Place, error) {
	var (
		id               string
		slug             *string
		name             string
		subtitle         string
		short            string
		description      string
		typeRaw          string
		layerRaw         string
		categoryRaw      *string
		district         string
		address          string
		lat              float64
		lon              float64
		distanceM        *float64
		hours            string
		priceFree        *bool
		priceLabel       string
		duration         string
		season           string
		accessRaw        []string
		howToGet         string
		facts            []string
		tags             []string
		photosRaw        []byte
		beachRaw         *string
		swimSeason       string
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
		&slug,
		&name,
		&subtitle,
		&short,
		&description,
		&typeRaw,
		&layerRaw,
		&categoryRaw,
		&district,
		&address,
		&lat,
		&lon,
		&distanceM,
		&hours,
		&priceFree,
		&priceLabel,
		&duration,
		&season,
		&accessRaw,
		&howToGet,
		&facts,
		&tags,
		&photosRaw,
		&beachRaw,
		&swimSeason,
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

	var category *models.CategoryCode
	if categoryRaw != nil {
		code, cErr := models.NewCategoryCode(*categoryRaw)
		if cErr != nil {
			return models.Place{}, fmt.Errorf("parse category from db: %w", cErr)
		}
		category = &code
	}

	access := make([]models.AccessMode, 0, len(accessRaw))
	for _, raw := range accessRaw {
		mode, mErr := models.NewAccessMode(raw)
		if mErr != nil {
			return models.Place{}, fmt.Errorf("parse access mode from db: %w", mErr)
		}
		access = append(access, mode)
	}

	var beach *models.BeachType
	if beachRaw != nil {
		parsed, bErr := models.NewBeachType(*beachRaw)
		if bErr != nil {
			return models.Place{}, fmt.Errorf("parse beach type from db: %w", bErr)
		}
		beach = &parsed
	}

	var photos []models.Photo
	if len(photosRaw) > 0 {
		if err := json.Unmarshal(photosRaw, &photos); err != nil {
			return models.Place{}, fmt.Errorf("decode place photos: %w", err)
		}
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
		Slug:            slug,
		Name:            name,
		Subtitle:        subtitle,
		Short:           short,
		Description:     description,
		Type:            pt,
		Layer:           layer,
		Category:        category,
		District:        district,
		Address:         address,
		Location:        loc,
		Hours:           hours,
		PriceFree:       priceFree,
		PriceLabel:      priceLabel,
		Duration:        duration,
		Season:          season,
		Access:          access,
		HowToGet:        howToGet,
		Facts:           facts,
		Tags:            tags,
		Photos:          photos,
		Beach:           beach,
		SwimSeason:      swimSeason,
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

	if filter.Category != nil {
		where = append(where, fmt.Sprintf("p.category = $%d", argPos))
		args = append(args, string(*filter.Category))
		argPos++
	}

	if filter.Q != nil && strings.TrimSpace(*filter.Q) != "" {
		where = append(where, fmt.Sprintf(`(
			p.name ILIKE $%[1]d
			OR p.subtitle ILIKE $%[1]d
			OR p.description ILIKE $%[1]d
			OR p.address ILIKE $%[1]d
			OR p.district ILIKE $%[1]d
			OR EXISTS (SELECT 1 FROM unnest(p.tags) tag WHERE tag ILIKE $%[1]d)
		)`, argPos))
		args = append(args, "%"+strings.TrimSpace(*filter.Q)+"%")
		argPos++
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultPlaceListLimit
	}
	args = append(args, limit)
	limitPos := argPos

	query := fmt.Sprintf(`
		SELECT %s
		FROM places p
		%s
	`, fmt.Sprintf(placeColumns, distSelect), latestAssessmentJoin)

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

func categoryValue(category *models.CategoryCode) *string {
	if category == nil {
		return nil
	}
	value := string(*category)

	return &value
}

func beachValue(beach *models.BeachType) *string {
	if beach == nil {
		return nil
	}
	value := string(*beach)

	return &value
}

func difficultyValue(difficulty *models.Difficulty) *int {
	if difficulty == nil {
		return nil
	}
	value := int(*difficulty)

	return &value
}

func photosValue(photos []models.Photo) []byte {
	if len(photos) == 0 {
		return []byte("[]")
	}
	raw, err := json.Marshal(photos)
	if err != nil {
		return []byte("[]")
	}

	return raw
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
