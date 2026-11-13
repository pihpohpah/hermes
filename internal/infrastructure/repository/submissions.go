package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

const defaultSubmissionListLimit = 50

// SubmissionRepository stores user proposals with their editorial status.
type SubmissionRepository struct {
	pool *pgxpool.Pool
}

func NewSubmissionRepository(pool *pgxpool.Pool) *SubmissionRepository {
	return &SubmissionRepository{pool: pool}
}

const submissionColumns = `
	id::text,
	author_id::text,
	type,
	status,
	scenario,
	category,
	title,
	address,
	ST_Y(location::geometry),
	ST_X(location::geometry),
	location IS NOT NULL,
	event_date,
	event_time,
	description,
	hours,
	price,
	link,
	fix_id,
	fix_reasons,
	contact,
	photos,
	moderator_note,
	created_at,
	updated_at
`

func (r *SubmissionRepository) Create(ctx context.Context, in models.CreateSubmissionInput) (models.Submission, error) {
	const query = `
		INSERT INTO submissions (
			author_id, idempotency_key, type, scenario, category, title, address,
			location, event_date, event_time, description, hours, price, link,
			fix_id, fix_reasons, contact, photos
		)
		VALUES (
			$1::uuid, $2, $3, $4, $5, $6, $7,
			CASE WHEN $8::double precision IS NULL THEN NULL
			     ELSE ST_SetSRID(ST_MakePoint($8, $9), 4326)::geography END,
			$10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19
		)
		RETURNING ` + submissionColumns

	var lon, lat *float64
	if in.Location != nil {
		lon = &in.Location.Lon
		lat = &in.Location.Lat
	}

	var eventDate *time.Time
	if in.EventDate != nil {
		d := *in.EventDate
		eventDate = &d
	}

	submission, err := scanSubmission(r.pool.QueryRow(
		ctx,
		query,
		in.AuthorID,
		nullIfEmpty(in.IdempotencyKey),
		string(in.Type),
		layerValue(in.Scenario),
		in.Category,
		in.Title,
		in.Address,
		lon,
		lat,
		eventDate,
		in.EventTime,
		in.Description,
		in.Hours,
		in.Price,
		in.Link,
		in.FixID,
		in.FixReasons,
		in.Contact,
		photosValue(in.Photos),
	))
	if err != nil {
		return models.Submission{}, fmt.Errorf("insert submission: %w", err)
	}

	return submission, nil
}

// GetByIDAndAuthor loads one submission; another author's entry is
// indistinguishable from a missing one (404).
func (r *SubmissionRepository) GetByIDAndAuthor(ctx context.Context, id, authorID string) (models.Submission, error) {
	const query = `
		SELECT ` + submissionColumns + `
		FROM submissions
		WHERE id = $1::uuid AND author_id = $2::uuid
	`

	submission, err := scanSubmission(r.pool.QueryRow(ctx, query, id, authorID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Submission{}, models.ErrNotFound
		}

		return models.Submission{}, fmt.Errorf("query submission by id: %w", err)
	}

	return submission, nil
}

// GetByAuthorAndKey resolves the submission pinned to an idempotency key (replay path).
func (r *SubmissionRepository) GetByAuthorAndKey(
	ctx context.Context,
	authorID, key string,
) (models.Submission, error) {
	const query = `
		SELECT ` + submissionColumns + `
		FROM submissions
		WHERE author_id = $1::uuid AND idempotency_key = $2
		ORDER BY created_at DESC
		LIMIT 1
	`

	submission, err := scanSubmission(r.pool.QueryRow(ctx, query, authorID, key))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Submission{}, models.ErrNotFound
		}

		return models.Submission{}, fmt.Errorf("query submission by idempotency key: %w", err)
	}

	return submission, nil
}

func (r *SubmissionRepository) ListByAuthor(ctx context.Context, filter models.SubmissionFilter) ([]models.Submission, error) {
	var (
		where = []string{"author_id = $1::uuid"}
		args  = []any{filter.AuthorID}
	)

	if filter.Status != nil {
		where = append(where, fmt.Sprintf("status = $%d", len(args)+1))
		args = append(args, string(*filter.Status))
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultSubmissionListLimit
	}
	args = append(args, limit)

	query := fmt.Sprintf(`
		SELECT %s
		FROM submissions
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d
	`, submissionColumns, joinClauses(where), len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query submissions: %w", err)
	}
	defer rows.Close()

	result := make([]models.Submission, 0)
	for rows.Next() {
		submission, scanErr := scanSubmission(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, submission)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate submissions rows: %w", err)
	}

	return result, nil
}

func scanSubmission(row rowScanner) (models.Submission, error) {
	var (
		submission  models.Submission
		typeRaw     string
		statusRaw   string
		scenarioRaw *string
		lat         *float64
		lon         *float64
		hasLocation bool
		eventDate   *time.Time
		fixReasons  []string
		photosRaw   []byte
		createdAt   time.Time
		updatedAt   time.Time
	)

	if err := row.Scan(
		&submission.ID,
		&submission.AuthorID,
		&typeRaw,
		&statusRaw,
		&scenarioRaw,
		&submission.Category,
		&submission.Title,
		&submission.Address,
		&lat,
		&lon,
		&hasLocation,
		&eventDate,
		&submission.EventTime,
		&submission.Description,
		&submission.Hours,
		&submission.Price,
		&submission.Link,
		&submission.FixID,
		&fixReasons,
		&submission.Contact,
		&photosRaw,
		&submission.ModeratorNote,
		&createdAt,
		&updatedAt,
	); err != nil {
		return models.Submission{}, fmt.Errorf("scan submission row: %w", err)
	}

	submissionType, err := models.NewSubmissionType(typeRaw)
	if err != nil {
		return models.Submission{}, fmt.Errorf("parse submission type from db: %w", err)
	}
	status, err := models.NewSubmissionStatus(statusRaw)
	if err != nil {
		return models.Submission{}, fmt.Errorf("parse submission status from db: %w", err)
	}

	submission.Type = submissionType
	submission.Status = status
	submission.FixReasons = fixReasons
	submission.CreatedAt = createdAt.UTC()
	submission.UpdatedAt = updatedAt.UTC()

	if scenarioRaw != nil {
		layer, err := models.NewLayerCode(*scenarioRaw)
		if err != nil {
			return models.Submission{}, fmt.Errorf("parse submission scenario from db: %w", err)
		}
		submission.Scenario = &layer
	}
	if hasLocation && lat != nil && lon != nil {
		location, err := models.NewPoint(*lat, *lon)
		if err != nil {
			return models.Submission{}, fmt.Errorf("parse submission location: %w", err)
		}
		submission.Location = &location
	}
	if eventDate != nil {
		d := eventDate.UTC()
		submission.EventDate = &d
	}
	if len(photosRaw) > 0 {
		if err := json.Unmarshal(photosRaw, &submission.Photos); err != nil {
			return models.Submission{}, fmt.Errorf("decode submission photos: %w", err)
		}
	}

	return submission, nil
}

func layerValue(layer *models.LayerCode) *string {
	if layer == nil {
		return nil
	}
	value := string(*layer)

	return &value
}

func nullIfEmpty(raw string) *string {
	if raw == "" {
		return nil
	}

	return &raw
}

func joinClauses(clauses []string) string {
	var result string
	for i, clause := range clauses {
		if i > 0 {
			result += " AND "
		}
		result += clause
	}

	return result
}
