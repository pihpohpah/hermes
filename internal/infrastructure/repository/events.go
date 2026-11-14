package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

const defaultEventListLimit = 50

// EventRepository stores dated catalog events.
type EventRepository struct {
	pool *pgxpool.Pool
}

func NewEventRepository(pool *pgxpool.Pool) *EventRepository {
	return &EventRepository{pool: pool}
}

const eventColumns = `
	e.id::text,
	e.slug,
	e.title,
	e.category,
	e.short,
	e.description,
	e.place_id::text,
	p.name,
	e.venue,
	e.organizer,
	e.age,
	e.starts_at,
	e.ends_at,
	e.created_at
`

func (r *EventRepository) List(ctx context.Context, filter models.EventFilter) ([]models.Event, error) {
	var (
		where []string
		args  = []any{filter.Now}
	)

	// $1 is now; upcoming events are the default catalog view.
	if filter.UpcomingOnly {
		where = append(where, "e.ends_at > $1")
	}

	if filter.PlaceID != nil {
		where = append(where, fmt.Sprintf("e.place_id = $%d::uuid", len(args)+1))
		args = append(args, *filter.PlaceID)
	}

	if filter.Category != nil {
		where = append(where, fmt.Sprintf("e.category = $%d", len(args)+1))
		args = append(args, string(*filter.Category))
	}

	if q := strings.TrimSpace(filter.Q); q != "" {
		where = append(where, fmt.Sprintf(`(
			e.title ILIKE $%[1]d
			OR e.short ILIKE $%[1]d
			OR e.description ILIKE $%[1]d
			OR e.venue ILIKE $%[1]d
			OR e.organizer ILIKE $%[1]d
		)`, len(args)+1))
		args = append(args, "%"+q+"%")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultEventListLimit
	}
	args = append(args, limit)

	query := fmt.Sprintf(`
		SELECT %s
		FROM events e
		LEFT JOIN places p ON p.id = e.place_id
	`, eventColumns)
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY e.starts_at ASC LIMIT $%d", len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	result := make([]models.Event, 0)
	for rows.Next() {
		event, scanErr := scanEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events rows: %w", err)
	}

	return result, nil
}

// GetByID resolves an event by UUID or slug.
func (r *EventRepository) GetByID(ctx context.Context, id string) (models.Event, error) {
	const query = `
		SELECT ` + eventColumns + `
		FROM events e
		LEFT JOIN places p ON p.id = e.place_id
		WHERE e.id::text = $1 OR e.slug = $1
	`

	event, err := scanEvent(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Event{}, models.ErrNotFound
		}

		return models.Event{}, fmt.Errorf("query event by id: %w", err)
	}

	return event, nil
}

// GetManyByIDs loads a set of events for collection hydration.
func (r *EventRepository) GetManyByIDs(ctx context.Context, ids []string) ([]models.Event, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	const query = `
		SELECT ` + eventColumns + `
		FROM events e
		LEFT JOIN places p ON p.id = e.place_id
		WHERE e.id::text = ANY($1::text[])
	`

	rows, err := r.pool.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("query events by ids: %w", err)
	}
	defer rows.Close()

	result := make([]models.Event, 0, len(ids))
	for rows.Next() {
		event, scanErr := scanEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events by ids rows: %w", err)
	}

	return result, nil
}

func scanEvent(row rowScanner) (models.Event, error) {
	var (
		event     models.Event
		category  string
		placeName *string
		startsAt  time.Time
		endsAt    time.Time
		createdAt time.Time
	)

	if err := row.Scan(
		&event.ID,
		&event.Slug,
		&event.Title,
		&category,
		&event.Short,
		&event.Description,
		&event.PlaceID,
		&placeName,
		&event.Venue,
		&event.Organizer,
		&event.Age,
		&startsAt,
		&endsAt,
		&createdAt,
	); err != nil {
		return models.Event{}, fmt.Errorf("scan event row: %w", err)
	}

	parsedCategory, err := models.NewEventCategoryCode(category)
	if err != nil {
		return models.Event{}, fmt.Errorf("parse event category from db: %w", err)
	}

	event.Category = parsedCategory
	event.PlaceName = placeName
	event.StartsAt = startsAt.UTC()
	event.EndsAt = endsAt.UTC()
	event.CreatedAt = createdAt.UTC()

	return event, nil
}
