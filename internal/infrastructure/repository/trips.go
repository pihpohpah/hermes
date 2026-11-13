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

func (r *TripRepository) ListByUser(ctx context.Context, filter models.TripFilter) ([]models.Trip, error) {
	if filter.UserID == "" {
		return nil, models.Invalid("`user_id` is required")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultTripLimit
	}

	const query = `
		SELECT id::text, user_id::text, place_id::text, planned_at, activity, status, notes, created_at
		FROM trips
		WHERE user_id = $1::uuid
		ORDER BY planned_at ASC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, filter.UserID, limit)
	if err != nil {
		return nil, fmt.Errorf("query trips: %w", err)
	}
	defer rows.Close()

	result := make([]models.Trip, 0)
	for rows.Next() {
		trip, scanErr := scanTrip(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, trip)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trips rows: %w", err)
	}

	return result, nil
}

func (r *TripRepository) Create(ctx context.Context, in models.CreateTripInput) (models.Trip, error) {
	const query = `
		INSERT INTO trips (user_id, place_id, planned_at, activity, notes)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5)
		RETURNING id::text, user_id::text, place_id::text, planned_at, activity, status, notes, created_at
	`

	row := r.pool.QueryRow(ctx, query, in.UserID, in.PlaceID, in.PlannedAt, string(in.Activity), in.Notes)

	trip, err := scanTrip(row)
	if err != nil {
		return models.Trip{}, fmt.Errorf("create trip: %w", err)
	}

	return trip, nil
}

func (r *TripRepository) Update(ctx context.Context, in models.UpdateTripInput) (models.Trip, error) {
	const query = `
		UPDATE trips
		SET status = COALESCE($2, status),
		    notes  = COALESCE($3, notes)
		WHERE id = $1::uuid
		  AND user_id = $4::uuid
		RETURNING id::text, user_id::text, place_id::text, planned_at, activity, status, notes, created_at
	`

	var status *string
	if in.Status != nil {
		value := string(*in.Status)
		status = &value
	}

	row := r.pool.QueryRow(ctx, query, in.ID, status, in.Notes, in.UserID)

	trip, err := scanTrip(row)
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
		&trip.PlaceID,
		&plannedAt,
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
	trip.CreatedAt = createdAt

	return trip, nil
}
