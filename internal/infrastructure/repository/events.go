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
	e.created_at,
	ST_Y(e.location::geometry),
	ST_X(e.location::geometry),
	e.external_id,
	e.status,
	e.price_free,
	e.price_label,
	e.event_url,
	e.source_urls
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

// UpsertBySlug вставляет редакционное событие или обновляет его по slug —
// путь сида и будущих админских правок. В отличие от UpsertImported
// (импорт event-agent по external_id) пишет все редакционные поля: short,
// place_id, organizer и age. Повторный seed обновляет ту же строку.
func (r *EventRepository) UpsertBySlug(ctx context.Context, event models.Event) (models.Event, error) {
	if event.Slug == nil || *event.Slug == "" {
		return models.Event{}, errors.New("upsert event by slug: empty slug")
	}

	var lon, lat *float64
	if event.Location != nil {
		lon, lat = &event.Location.Lon, &event.Location.Lat
	}

	status := event.Status
	if status == "" {
		status = models.EventStatusScheduled
	}

	const query = `
		INSERT INTO events (
			slug, title, category, short, description,
			place_id, venue, organizer, age, status,
			price_free, price_label, starts_at, ends_at, location
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			CASE WHEN $15::float8 IS NULL THEN NULL
			     ELSE ST_SetSRID(ST_MakePoint($15::float8, $16::float8), 4326)::geography END)
		ON CONFLICT (slug) DO UPDATE SET
			title = EXCLUDED.title,
			category = EXCLUDED.category,
			short = EXCLUDED.short,
			description = EXCLUDED.description,
			place_id = EXCLUDED.place_id,
			venue = EXCLUDED.venue,
			organizer = EXCLUDED.organizer,
			age = EXCLUDED.age,
			status = EXCLUDED.status,
			price_free = EXCLUDED.price_free,
			price_label = EXCLUDED.price_label,
			starts_at = EXCLUDED.starts_at,
			ends_at = EXCLUDED.ends_at,
			location = COALESCE(EXCLUDED.location, events.location)
		RETURNING id
	`

	var id string
	err := r.pool.QueryRow(
		ctx, query,
		*event.Slug,
		event.Title,
		string(event.Category),
		event.Short,
		event.Description,
		event.PlaceID,
		event.Venue,
		event.Organizer,
		event.Age,
		string(status),
		event.PriceFree,
		event.PriceLabel,
		event.StartsAt,
		event.EndsAt,
		lon,
		lat,
	).Scan(&id)
	if err != nil {
		return models.Event{}, fmt.Errorf("upsert event by slug: %w", err)
	}

	return r.GetByID(ctx, id)
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
		lat       *float64
		lon       *float64
		external  *string
		status    string
		priceFree *bool
		eventURL  *string
		sources   map[string]string
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
		&lat,
		&lon,
		&external,
		&status,
		&priceFree,
		&event.PriceLabel,
		&eventURL,
		&sources,
	); err != nil {
		return models.Event{}, fmt.Errorf("scan event row: %w", err)
	}

	parsedCategory, err := models.NewEventCategoryCode(category)
	if err != nil {
		return models.Event{}, fmt.Errorf("parse event category from db: %w", err)
	}

	parsedStatus, err := models.NewEventStatus(status)
	if err != nil {
		return models.Event{}, fmt.Errorf("parse event status from db: %w", err)
	}

	if lat != nil && lon != nil {
		point, pointErr := models.NewPoint(*lat, *lon)
		if pointErr != nil {
			return models.Event{}, fmt.Errorf("parse event location from db: %w", pointErr)
		}
		event.Location = &point
	}

	event.Category = parsedCategory
	event.Status = parsedStatus
	event.PlaceName = placeName
	event.ExternalID = external
	event.PriceFree = priceFree
	event.EventURL = eventURL
	event.SourceURLs = sources
	event.StartsAt = startsAt.UTC()
	event.EndsAt = endsAt.UTC()
	event.CreatedAt = createdAt.UTC()

	return event, nil
}
