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

// IdempotencyRepository pins mutating-request outcomes to client keys.
type IdempotencyRepository struct {
	pool *pgxpool.Pool
}

func NewIdempotencyRepository(pool *pgxpool.Pool) *IdempotencyRepository {
	return &IdempotencyRepository{pool: pool}
}

// Get returns the live record for (scope, key). Expired records are treated
// as absent so a key can be reused after its TTL.
func (r *IdempotencyRepository) Get(
	ctx context.Context,
	scope, key string,
) (models.IdempotencyRecord, error) {
	const query = `
		SELECT request_fingerprint, user_id::text, expires_at
		FROM idempotency_keys
		WHERE scope = $1 AND key = $2 AND expires_at > now()
	`

	var record models.IdempotencyRecord

	err := queryWith(ctx, r.pool).QueryRow(ctx, query, scope, key).Scan(
		&record.RequestFingerprint,
		&record.UserID,
		&record.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.IdempotencyRecord{}, models.ErrNotFound
		}

		return models.IdempotencyRecord{}, fmt.Errorf("select idempotency record: %w", err)
	}

	record.Scope = scope
	record.Key = key

	return record, nil
}

// Save inserts the record. The second return value reports whether the insert
// happened; false means another request already claimed the (scope, key) pair.
// The conflicting row is left untouched, so the caller can re-read it.
func (r *IdempotencyRepository) Save(
	ctx context.Context,
	record models.IdempotencyRecord,
) (bool, error) {
	const query = `
		INSERT INTO idempotency_keys (scope, key, request_fingerprint, user_id, expires_at)
		VALUES ($1, $2, $3, $4::uuid, $5)
		ON CONFLICT (scope, key) DO NOTHING
		RETURNING scope
	`

	var insertedScope string

	err := queryWith(ctx, r.pool).QueryRow(ctx, query,
		record.Scope,
		record.Key,
		record.RequestFingerprint,
		record.UserID,
		record.ExpiresAt,
	).Scan(&insertedScope)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}

		return false, fmt.Errorf("insert idempotency record: %w", err)
	}

	return true, nil
}

// CleanupExpired removes finished keys. It is exported for console use and
// future periodic maintenance; API flows never need it because Get ignores
// expired rows.
func (r *IdempotencyRepository) CleanupExpired(ctx context.Context, now time.Time) (int64, error) {
	const query = `DELETE FROM idempotency_keys WHERE expires_at <= $1`

	tag, err := queryWith(ctx, r.pool).Exec(ctx, query, now)
	if err != nil {
		return 0, fmt.Errorf("delete expired idempotency records: %w", err)
	}

	return tag.RowsAffected(), nil
}
