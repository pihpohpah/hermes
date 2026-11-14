package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// PhotoRepository stores photo assets with moderation state and attribution.
type PhotoRepository struct {
	pool *pgxpool.Pool
}

func NewPhotoRepository(pool *pgxpool.Pool) *PhotoRepository {
	return &PhotoRepository{pool: pool}
}

const photoColumns = `
	id::text,
	owner_id::text,
	status,
	storage_path,
	original_name,
	content_type,
	size_bytes,
	width,
	height,
	sha256,
	alt,
	author,
	license,
	license_url,
	source_url,
	place_id::text,
	submission_id::text,
	moderated_by::text,
	moderated_at,
	created_at,
	updated_at
`

func (r *PhotoRepository) Create(ctx context.Context, in models.CreatePhotoInput) (models.StoredPhoto, error) {
	id := in.ID
	if id == "" {
		var err error
		if id, err = models.NewUUIDv7(time.Now()); err != nil {
			return models.StoredPhoto{}, err
		}
	}

	status := in.Status
	if status == "" {
		status = models.PhotoStatusModeration
	}

	const query = `
		INSERT INTO photos (id, owner_id, status, storage_path, original_name, content_type, size_bytes, width, height, sha256)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING ` + photoColumns

	photo, err := scanPhoto(r.pool.QueryRow(
		ctx,
		query,
		id,
		in.OwnerID,
		status,
		in.StoragePath,
		in.OriginalName,
		in.ContentType,
		in.SizeBytes,
		in.Width,
		in.Height,
		in.SHA256,
	))
	if err != nil {
		return models.StoredPhoto{}, fmt.Errorf("insert photo: %w", err)
	}

	return photo, nil
}

func scanPhoto(row rowScanner) (models.StoredPhoto, error) {
	var (
		photo     models.StoredPhoto
		statusRaw string
		createdAt time.Time
		updatedAt time.Time
	)

	if err := row.Scan(
		&photo.ID,
		&photo.OwnerID,
		&statusRaw,
		&photo.StoragePath,
		&photo.OriginalName,
		&photo.ContentType,
		&photo.SizeBytes,
		&photo.Width,
		&photo.Height,
		&photo.SHA256,
		&photo.Alt,
		&photo.Author,
		&photo.License,
		&photo.LicenseURL,
		&photo.SourceURL,
		&photo.PlaceID,
		&photo.SubmissionID,
		&photo.ModeratedBy,
		&photo.ModeratedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return models.StoredPhoto{}, fmt.Errorf("scan photo row: %w", err)
	}

	status, err := models.NewPhotoStatus(statusRaw)
	if err != nil {
		return models.StoredPhoto{}, fmt.Errorf("parse photo status from db: %w", err)
	}

	photo.Status = status
	photo.CreatedAt = createdAt.UTC()
	photo.UpdatedAt = updatedAt.UTC()

	return photo, nil
}
