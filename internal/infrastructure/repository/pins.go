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

const defaultPinListLimit = 200

// PinRepository stores radar pins: ephemeral activity markers created by
// users and visible on the map until ends_at.
type PinRepository struct {
	pool *pgxpool.Pool
}

func NewPinRepository(pool *pgxpool.Pool) *PinRepository {
	return &PinRepository{pool: pool}
}

// pinColumns carries the pin row plus aggregate viewer decoration: the
// number of goings, whether the viewer joined, and whether the pin is the
// viewer's own. $1 is the viewer user id (NULL for anonymous).
const pinColumns = `
	p.id::text,
	p.kind,
	p.title,
	p.note,
	p.place_label,
	ST_Y(p.location::geometry) AS lat,
	ST_X(p.location::geometry) AS lon,
	p.starts_at,
	p.ends_at,
	p.need,
	p.author_id::text,
	u.display_name,
	p.created_at,
	(SELECT count(*) FROM pin_goings g WHERE g.pin_id = p.id) AS going,
	EXISTS(SELECT 1 FROM pin_goings g WHERE g.pin_id = p.id AND g.user_id::text = $1) AS joined,
	COALESCE(p.author_id::text = $1, false) AS mine
`

func (r *PinRepository) ListActive(ctx context.Context, filter models.PinFilter) ([]models.Pin, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultPinListLimit
	}

	var (
		where []string
		args  = []any{filter.Viewer, filter.Now}
	)

	// $1 viewer, $2 now.
	where = append(where, "p.ends_at > $2", "p.hidden = false")

	if filter.Bounds != nil {
		where = append(where, "p.location && ST_MakeEnvelope($3, $4, $5, $6, 4326)")
		args = append(args,
			filter.Bounds.MinLon, filter.Bounds.MinLat,
			filter.Bounds.MaxLon, filter.Bounds.MaxLat,
		)
	}

	if filter.Kind != nil {
		where = append(where, fmt.Sprintf("p.kind = $%d", len(args)+1))
		args = append(args, string(*filter.Kind))
	}

	args = append(args, limit)

	query := fmt.Sprintf(`
		SELECT %s
		FROM pins p
		JOIN users u ON u.id = p.author_id
		WHERE %s
		ORDER BY p.starts_at ASC
		LIMIT $%d
	`, pinColumns, strings.Join(where, " AND "), len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query active pins: %w", err)
	}
	defer rows.Close()

	result := make([]models.Pin, 0)
	for rows.Next() {
		pin, scanErr := scanPin(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, pin)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pins rows: %w", err)
	}

	return result, nil
}

// GetVisible returns one pin: active and not hidden, or the author's own
// (so the author can still see/delete a hidden pin).
func (r *PinRepository) GetVisible(ctx context.Context, id string, viewer *string, now time.Time) (models.Pin, error) {
	const query = `
		SELECT ` + pinColumns + `
		FROM pins p
		JOIN users u ON u.id = p.author_id
		WHERE p.id::text = $2
			AND (p.ends_at > $3 OR p.author_id::text = $1)
			AND (p.hidden = false OR p.author_id::text = $1)
	`

	pin, err := scanPin(r.pool.QueryRow(ctx, query, viewer, id, now))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Pin{}, models.ErrNotFound
		}

		return models.Pin{}, fmt.Errorf("query pin by id: %w", err)
	}

	return pin, nil
}

// GetAny returns a pin row regardless of visibility: mutations that carry
// their own authorization (delete by author) need the row, not the view.
func (r *PinRepository) GetAny(ctx context.Context, id string, viewer *string) (models.Pin, error) {
	const query = `
		SELECT ` + pinColumns + `
		FROM pins p
		JOIN users u ON u.id = p.author_id
		WHERE p.id::text = $2
	`

	pin, err := scanPin(r.pool.QueryRow(ctx, query, viewer, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Pin{}, models.ErrNotFound
		}

		return models.Pin{}, fmt.Errorf("query pin by id: %w", err)
	}

	return pin, nil
}

func (r *PinRepository) Create(ctx context.Context, in models.CreatePinInput) (models.Pin, error) {
	const query = `
		INSERT INTO pins (author_id, kind, title, note, place_label, location, starts_at, ends_at, need)
		VALUES ($1::uuid, $2, $3, $4, $5, ST_SetSRID(ST_MakePoint($6, $7), 4326)::geography, $8, $9, $10)
		RETURNING id::text`

	var createdID string
	if err := r.pool.QueryRow(
		ctx,
		query,
		in.AuthorID,
		string(in.Kind),
		in.Title,
		in.Note,
		in.PlaceLabel,
		in.Location.Lon,
		in.Location.Lat,
		in.StartsAt.UTC(),
		in.EndsAt.UTC(),
		in.Need,
	).Scan(&createdID); err != nil {
		return models.Pin{}, fmt.Errorf("insert pin: %w", err)
	}

	// Полный ряд с автором и декорациями читается отдельно: pinColumns
	// содержит алиасы и JOIN, недоступные в RETURNING.
	pin, err := r.GetAny(ctx, createdID, &in.AuthorID)
	if err != nil {
		return models.Pin{}, fmt.Errorf("load created pin: %w", err)
	}

	return pin, nil
}

// DeleteByAuthor removes a pin only when it belongs to the author; a missing
// row or someone else's pin are indistinguishable to the caller (404).
func (r *PinRepository) DeleteByAuthor(ctx context.Context, id, authorID string) error {
	const query = `DELETE FROM pins WHERE id::text = $1 AND author_id = $2::uuid`

	tag, err := r.pool.Exec(ctx, query, id, authorID)
	if err != nil {
		return fmt.Errorf("delete pin: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrNotFound
	}

	return nil
}

// SetGoing records or removes one user's "Я приду" on a live pin.
func (r *PinRepository) SetGoing(ctx context.Context, in models.SetGoingInput) error {
	var err error

	if in.Going {
		const query = `
			INSERT INTO pin_goings (pin_id, user_id)
			VALUES ($1::uuid, $2::uuid)
			ON CONFLICT (pin_id, user_id) DO NOTHING
		`
		_, err = r.pool.Exec(ctx, query, in.PinID, in.UserID)
	} else {
		const query = `DELETE FROM pin_goings WHERE pin_id = $1::uuid AND user_id = $2::uuid`
		_, err = r.pool.Exec(ctx, query, in.PinID, in.UserID)
	}
	if err != nil {
		return fmt.Errorf("set pin going: %w", err)
	}

	return nil
}

// CountActiveByAuthor limits how many live pins one user may hold.
func (r *PinRepository) CountActiveByAuthor(ctx context.Context, authorID string, now time.Time) (int, error) {
	const query = `SELECT count(*) FROM pins WHERE author_id = $1::uuid AND ends_at > $2`

	var count int
	if err := r.pool.QueryRow(ctx, query, authorID, now).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active pins by author: %w", err)
	}

	return count, nil
}

// CountRecentByAuthor is the rate limit: creations in the trailing window.
func (r *PinRepository) CountRecentByAuthor(ctx context.Context, authorID string, since time.Time) (int, error) {
	const query = `SELECT count(*) FROM pins WHERE author_id = $1::uuid AND created_at > $2`

	var count int
	if err := r.pool.QueryRow(ctx, query, authorID, since).Scan(&count); err != nil {
		return 0, fmt.Errorf("count recent pins by author: %w", err)
	}

	return count, nil
}

// CreateReport stores one complaint per (pin, user) and hides the pin once
// the report count reaches threshold. The threshold check runs in the same
// statement group, so repeated reports cannot race past it.
func (r *PinRepository) CreateReport(
	ctx context.Context,
	in models.CreatePinReportInput,
	hideThreshold int,
) (models.PinReport, error) {
	const insertQuery = `
		INSERT INTO pin_reports (pin_id, user_id, reason)
		VALUES ($1::uuid, $2::uuid, $3)
		ON CONFLICT (pin_id, user_id) DO UPDATE SET reason = EXCLUDED.reason
		RETURNING id::text, pin_id::text, user_id::text, reason, created_at
	`

	var report models.PinReport
	if err := r.pool.QueryRow(
		ctx, insertQuery, in.PinID, in.UserID, in.Reason,
	).Scan(&report.ID, &report.PinID, &report.UserID, &report.Reason, &report.CreatedAt); err != nil {
		return models.PinReport{}, fmt.Errorf("insert pin report: %w", err)
	}

	if hideThreshold > 0 {
		const hideQuery = `
			UPDATE pins
			SET hidden = true,
			    hidden_reason = 'reports'
			WHERE id = $1::uuid
				AND hidden = false
				AND (SELECT count(*) FROM pin_reports WHERE pin_id = $1::uuid) >= $2
		`
		if _, err := r.pool.Exec(ctx, hideQuery, in.PinID, hideThreshold); err != nil {
			return models.PinReport{}, fmt.Errorf("apply pin report threshold: %w", err)
		}
	}

	return report, nil
}

// PurgeExpired physically deletes pins whose grace period has passed.
// Goings and reports cascade.
func (r *PinRepository) PurgeExpired(ctx context.Context, grace time.Duration) (int64, error) {
	const query = `DELETE FROM pins WHERE ends_at < now() - make_interval(secs => $1)`

	tag, err := r.pool.Exec(ctx, query, grace.Seconds())
	if err != nil {
		return 0, fmt.Errorf("purge expired pins: %w", err)
	}

	return tag.RowsAffected(), nil
}

func scanPin(row rowScanner) (models.Pin, error) {
	var (
		pin       models.Pin
		kindRaw   string
		lat       float64
		lon       float64
		startsAt  time.Time
		endsAt    time.Time
		createdAt time.Time
		going     int
		joined    bool
		mine      bool
	)

	if err := row.Scan(
		&pin.ID,
		&kindRaw,
		&pin.Title,
		&pin.Note,
		&pin.PlaceLabel,
		&lat,
		&lon,
		&startsAt,
		&endsAt,
		&pin.Need,
		&pin.AuthorID,
		&pin.AuthorName,
		&createdAt,
		&going,
		&joined,
		&mine,
	); err != nil {
		return models.Pin{}, fmt.Errorf("scan pin row: %w", err)
	}

	kind, err := models.NewPinKind(kindRaw)
	if err != nil {
		return models.Pin{}, fmt.Errorf("parse pin kind from db: %w", err)
	}
	location, err := models.NewPoint(lat, lon)
	if err != nil {
		return models.Pin{}, fmt.Errorf("parse pin location: %w", err)
	}

	pin.Kind = kind
	pin.Location = location
	pin.StartsAt = startsAt.UTC()
	pin.EndsAt = endsAt.UTC()
	pin.CreatedAt = createdAt.UTC()
	pin.Going = going
	pin.Joined = joined
	pin.Mine = mine
	pin.GoingTotal = going + 1 // the author always attends

	return pin, nil
}
