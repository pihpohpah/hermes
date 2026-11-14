package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

const defaultTripLimit = 50

type TripRepository struct {
	pool *pgxpool.Pool
}

func NewTripRepository(pool *pgxpool.Pool) *TripRepository {
	return &TripRepository{pool: pool}
}

func (r *TripRepository) Create(ctx context.Context, in models.CreateTripInput) (models.Trip, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return models.Trip{}, fmt.Errorf("begin trip create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const headerQuery = `
		INSERT INTO trips (user_id, planned_at, stop_minutes, activity, notes)
		VALUES ($1::uuid, $2, $3, $4, $5)
		RETURNING id::text, user_id::text, planned_at, stop_minutes, activity, status, notes, created_at
	`

	trip, err := scanTrip(tx.QueryRow(ctx, headerQuery,
		in.UserID, in.PlannedAt.UTC(), in.StopMinutes, string(in.Activity), in.Notes))
	if err != nil {
		return models.Trip{}, fmt.Errorf("create trip: %w", err)
	}

	const stopQuery = `
		INSERT INTO trip_stops (trip_id, place_id, position, arrive_at)
		VALUES ($1::uuid, $2::uuid, $3, $4)
	`

	for _, stop := range in.Stops {
		if _, err := tx.Exec(ctx, stopQuery, trip.ID, stop.PlaceID, stop.Position, stop.ArriveAt.UTC()); err != nil {
			return models.Trip{}, fmt.Errorf("create trip stop `%s`: %w", stop.PlaceID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return models.Trip{}, fmt.Errorf("commit trip create: %w", err)
	}

	trip.Stops = in.Stops

	return trip, nil
}

// ListByUserWithConditions returns the user's route plans ordered by planned
// time, every node enriched with the cached assessment closest to its arrival
// and the best hour of that day.
func (r *TripRepository) ListByUserWithConditions(
	ctx context.Context,
	filter models.TripFilter,
) ([]models.TripWithConditions, error) {
	if filter.UserID == "" {
		return nil, models.Invalid("`user_id` is required")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultTripLimit
	}

	const query = `
		SELECT id::text, user_id::text, planned_at, stop_minutes, activity, status, notes, created_at
		FROM trips
		WHERE user_id = $1::uuid
		ORDER BY planned_at ASC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, filter.UserID, limit)
	if err != nil {
		return nil, fmt.Errorf("query trips: %w", err)
	}

	headers := make([]models.Trip, 0)
	for rows.Next() {
		trip, scanErr := scanTrip(rows)
		if scanErr != nil {
			rows.Close()

			return nil, scanErr
		}
		headers = append(headers, trip)
	}
	if err := rows.Err(); err != nil {
		rows.Close()

		return nil, fmt.Errorf("iterate trips rows: %w", err)
	}
	rows.Close()

	return r.attachStops(ctx, headers)
}

// GetByIDWithConditions loads one trip with scored stops, owner-scoped.
func (r *TripRepository) GetByIDWithConditions(
	ctx context.Context,
	id string,
	userID string,
) (models.TripWithConditions, error) {
	const query = `
		SELECT id::text, user_id::text, planned_at, stop_minutes, activity, status, notes, created_at
		FROM trips
		WHERE id = $1::uuid AND user_id = $2::uuid
	`

	trip, err := scanTrip(r.pool.QueryRow(ctx, query, id, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.TripWithConditions{}, models.ErrNotFound
		}

		return models.TripWithConditions{}, fmt.Errorf("query trip: %w", err)
	}

	trips, err := r.attachStops(ctx, []models.Trip{trip})
	if err != nil {
		return models.TripWithConditions{}, err
	}

	return trips[0], nil
}

// attachStops loads ordered stops for the given trip headers and enriches
// every node with weather scoring in one query.
func (r *TripRepository) attachStops(
	ctx context.Context,
	headers []models.Trip,
) ([]models.TripWithConditions, error) {
	result := make([]models.TripWithConditions, 0, len(headers))
	if len(headers) == 0 {
		return result, nil
	}

	ids := make([]string, 0, len(headers))
	byID := make(map[string]*models.TripWithConditions, len(headers))
	for i := range headers {
		ids = append(ids, headers[i].ID)
		result = append(result, models.TripWithConditions{Trip: headers[i], Stops: []models.TripStopConditions{}})
		byID[headers[i].ID] = &result[i]
	}

	const stopsQuery = `
		SELECT s.trip_id::text, s.place_id::text, s.position, s.arrive_at,
		       p.name, COALESCE(p.slug, ''), p.district, t.activity,
		       ns.score, ns.verdict, ns.confidence, ns.forecast_time, ns.data, ns.factors,
		       ns.contributions, ns.warnings, ns.limitations, ns.summary, ns.sources, ns.demo, ns.algorithm_version,
		       bs.score, bs.verdict, bs.confidence, bs.forecast_time, bs.data, bs.factors,
		       bs.contributions, bs.warnings, bs.limitations, bs.summary, bs.sources, bs.demo, bs.algorithm_version
		FROM trip_stops s
		JOIN trips t ON t.id = s.trip_id
		JOIN places p ON p.id = s.place_id
		LEFT JOIN LATERAL (
			SELECT c.score, c.verdict, c.confidence, c.forecast_time, c.data, c.factors,
			       c.contributions, c.warnings, c.limitations, c.summary, c.sources, c.demo, c.algorithm_version
			FROM conditions_cache c
			WHERE c.place_id = s.place_id AND c.activity = t.activity
			ORDER BY abs(extract(epoch FROM c.forecast_time - s.arrive_at))
			LIMIT 1
		) ns ON true
		LEFT JOIN LATERAL (
			SELECT c.score, c.verdict, c.confidence, c.forecast_time, c.data, c.factors,
			       c.contributions, c.warnings, c.limitations, c.summary, c.sources, c.demo, c.algorithm_version
			FROM conditions_cache c
			WHERE c.place_id = s.place_id AND c.activity = t.activity
			  AND c.forecast_time >= (date_trunc('day', s.arrive_at AT TIME ZONE 'Asia/Vladivostok') AT TIME ZONE 'Asia/Vladivostok')
			  AND c.forecast_time <  (date_trunc('day', s.arrive_at AT TIME ZONE 'Asia/Vladivostok') AT TIME ZONE 'Asia/Vladivostok') + interval '1 day'
			ORDER BY c.score DESC, c.forecast_time ASC
			LIMIT 1
		) bs ON true
		WHERE s.trip_id = ANY($1::uuid[])
		ORDER BY s.trip_id, s.position ASC
	`

	rows, err := r.pool.Query(ctx, stopsQuery, ids)
	if err != nil {
		return nil, fmt.Errorf("query trip stops: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		stop, err := scanTripStopConditions(rows)
		if err != nil {
			return nil, err
		}

		owner, ok := byID[stop.ownerID]
		if !ok {
			continue
		}
		owner.Stops = append(owner.Stops, stop.TripStopConditions)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trip stops rows: %w", err)
	}

	return result, nil
}

type tripStopRow struct {
	ownerID string
	models.TripStopConditions
}

func scanTripStopConditions(row pgx.Row) (tripStopRow, error) {
	var (
		out      tripStopRow
		activity string
		arriveAt time.Time
		near     nullableAssessment
		best     nullableAssessment
	)

	if err := row.Scan(
		&out.ownerID,
		&out.PlaceID,
		&out.Position,
		&arriveAt,
		&out.PlaceName,
		&out.PlaceSlug,
		&out.District,
		&activity,
		&near.score, &near.verdict, &near.confidence, &near.forecastTime, &near.rawData, &near.factors,
		&near.contributions, &near.warnings, &near.limitations, &near.summary, &near.sources, &near.demo, &near.algorithm,
		&best.score, &best.verdict, &best.confidence, &best.forecastTime, &best.rawData, &best.factors,
		&best.contributions, &best.warnings, &best.limitations, &best.summary, &best.sources, &best.demo, &best.algorithm,
	); err != nil {
		return tripStopRow{}, fmt.Errorf("scan trip stop: %w", err)
	}

	out.ArriveAt = arriveAt
	out.Warnings = []models.Warning{}

	parsedActivity, err := models.NewActivity(activity)
	if err != nil {
		return tripStopRow{}, fmt.Errorf("parse trip activity `%s`: %w", activity, err)
	}

	if near.score != nil {
		assessment, err := near.build(out.PlaceID, parsedActivity)
		if err != nil {
			return tripStopRow{}, err
		}

		score := assessment.Score
		verdict := assessment.Verdict
		at := assessment.ForecastTime
		out.Score = &score
		out.Verdict = &verdict
		out.Summary = assessment.Summary
		out.ScoreAt = &at
		out.Warnings = assessment.Warnings
	}

	if best.score != nil {
		assessment, err := best.build(out.PlaceID, parsedActivity)
		if err != nil {
			return tripStopRow{}, err
		}

		score := assessment.Score
		verdict := assessment.Verdict
		at := assessment.ForecastTime
		out.BestScore = &score
		out.BestVerdict = &verdict
		out.BestAt = &at
	}

	return out, nil
}

// nullableAssessment wraps raw conditions_cache columns for lateral joins:
// every field is a pointer so an empty join result scans as NULLs.
type nullableAssessment struct {
	score         *int
	verdict       *string
	confidence    *string
	forecastTime  *time.Time
	rawData       *[]byte
	factors       *[]byte
	contributions *[]byte
	warnings      *[]byte
	limitations   *[]byte
	summary       *string
	sources       *[]byte
	demo          *bool
	algorithm     *string
}

func (a nullableAssessment) build(placeID string, activity models.Activity) (models.ConditionAssessment, error) {
	assessment, err := buildAssessment(
		placeID,
		activity,
		a.forecastTime.UTC(),
		*a.score,
		*a.verdict,
		*a.confidence,
		valueOrNil(a.rawData),
		valueOrNil(a.factors),
		valueOrNil(a.contributions),
		valueOrNil(a.warnings),
		valueOrNil(a.limitations),
		*a.summary,
		valueOrNil(a.sources),
		*a.demo,
		*a.algorithm,
	)
	if err != nil {
		return models.ConditionAssessment{}, fmt.Errorf("decode trip stop assessment: %w", err)
	}

	return assessment, nil
}

func valueOrNil(raw *[]byte) []byte {
	if raw == nil {
		return nil
	}

	return *raw
}

func (r *TripRepository) Update(ctx context.Context, in models.UpdateTripInput) (models.Trip, error) {
	const query = `
		UPDATE trips
		SET status = COALESCE($2, status),
		    notes  = COALESCE($3, notes)
		WHERE id = $1::uuid
		  AND user_id = $4::uuid
		RETURNING id::text, user_id::text, planned_at, stop_minutes, activity, status, notes, created_at
	`

	var status *string
	if in.Status != nil {
		value := string(*in.Status)
		status = &value
	}

	trip, err := scanTrip(r.pool.QueryRow(ctx, query, in.ID, status, in.Notes, in.UserID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Trip{}, models.ErrNotFound
		}

		return models.Trip{}, fmt.Errorf("update trip: %w", err)
	}

	return trip, nil
}

func scanTrip(row rowScanner) (models.Trip, error) {
	var (
		trip      models.Trip
		activity  string
		status    string
		plannedAt time.Time
		createdAt time.Time
	)

	if err := row.Scan(
		&trip.ID,
		&trip.UserID,
		&plannedAt,
		&trip.StopMinutes,
		&activity,
		&status,
		&trip.Notes,
		&createdAt,
	); err != nil {
		return models.Trip{}, err
	}

	parsedActivity, err := models.NewActivity(activity)
	if err != nil {
		return models.Trip{}, fmt.Errorf("parse trip activity `%s`: %w", activity, err)
	}

	parsedStatus, err := models.NewTripStatus(status)
	if err != nil {
		return models.Trip{}, fmt.Errorf("parse trip status `%s`: %w", status, err)
	}

	trip.PlannedAt = plannedAt
	trip.Activity = parsedActivity
	trip.Status = parsedStatus
	trip.Stops = []models.TripStop{}
	trip.CreatedAt = createdAt

	return trip, nil
}
