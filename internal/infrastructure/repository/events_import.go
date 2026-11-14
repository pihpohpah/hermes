package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// UpsertImported вставляет событие импорта или обновляет его по стабильному
// external_id. Возвращает true, когда вставила новую строку. Координаты не
// затираются пустым значением: старый геокодинг переживает прогон, в котором
// адрес не прогеокодировался.
func (r *EventRepository) UpsertImported(ctx context.Context, event models.Event) (bool, error) {
	if event.ExternalID == nil || *event.ExternalID == "" {
		return false, errors.New("upsert imported event: empty external id")
	}

	var lon, lat *float64
	if event.Location != nil {
		lon, lat = &event.Location.Lon, &event.Location.Lat
	}

	status := event.Status
	if status == "" {
		status = models.EventStatusScheduled
	}
	sources := event.SourceURLs
	if sources == nil {
		sources = map[string]string{}
	}

	const query = `
		INSERT INTO events (
			title, category, description, venue, status,
			price_free, price_label, event_url, source_urls,
			starts_at, ends_at, external_id, location
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			CASE WHEN $13 IS NULL THEN NULL
			     ELSE ST_SetSRID(ST_MakePoint($13, $14), 4326)::geography END)
		ON CONFLICT (external_id) WHERE external_id IS NOT NULL
		DO UPDATE SET
			title = EXCLUDED.title,
			category = EXCLUDED.category,
			description = EXCLUDED.description,
			venue = EXCLUDED.venue,
			status = EXCLUDED.status,
			price_free = EXCLUDED.price_free,
			price_label = EXCLUDED.price_label,
			event_url = EXCLUDED.event_url,
			source_urls = EXCLUDED.source_urls,
			starts_at = EXCLUDED.starts_at,
			ends_at = EXCLUDED.ends_at,
			location = COALESCE(EXCLUDED.location, events.location)
		RETURNING xmax = 0 AS inserted
	`

	var inserted bool
	err := r.pool.QueryRow(
		ctx, query,
		event.Title,
		string(event.Category),
		event.Description,
		event.Venue,
		string(status),
		event.PriceFree,
		event.PriceLabel,
		event.EventURL,
		sources,
		event.StartsAt,
		event.EndsAt,
		*event.ExternalID,
		lon,
		lat,
	).Scan(&inserted)
	if err != nil {
		return false, fmt.Errorf("upsert imported event `%s`: %w", *event.ExternalID, err)
	}

	return inserted, nil
}
