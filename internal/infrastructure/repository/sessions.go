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

type SessionRepository struct {
	pool *pgxpool.Pool
}

func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

func (r *SessionRepository) Create(ctx context.Context, session models.UserSession) (models.UserSession, error) {
	const query = `
		INSERT INTO user_sessions (user_id, token_hash, csrf_hash, expires_at)
		VALUES ($1::uuid, $2, $3, $4)
		RETURNING id::text, created_at, expires_at
	`

	var (
		stored    models.UserSession
		createdAt time.Time
		expiresAt time.Time
	)

	err := queryWith(ctx, r.pool).QueryRow(ctx, query,
		session.UserID, session.TokenHash, session.CSRFHash, session.ExpiresAt,
	).Scan(
		&stored.ID,
		&createdAt,
		&expiresAt,
	)
	if err != nil {
		return models.UserSession{}, fmt.Errorf("insert user session: %w", err)
	}

	stored.UserID = session.UserID
	stored.TokenHash = session.TokenHash
	stored.CSRFHash = session.CSRFHash
	stored.CreatedAt = createdAt
	stored.ExpiresAt = expiresAt

	return stored, nil
}

// FindActiveByTokenHash resolves a session by the hash of the opaque cookie
// token. Expired and revoked sessions are not returned, so the middleware can
// treat them as "no session".
func (r *SessionRepository) FindActiveByTokenHash(
	ctx context.Context,
	tokenHash string,
	now time.Time,
) (models.UserSession, error) {
	const query = `
		SELECT id::text, user_id::text, token_hash, csrf_hash, created_at, expires_at, revoked_at
		FROM user_sessions
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > $2
	`

	var session models.UserSession

	err := queryWith(ctx, r.pool).QueryRow(ctx, query, tokenHash, now).Scan(
		&session.ID,
		&session.UserID,
		&session.TokenHash,
		&session.CSRFHash,
		&session.CreatedAt,
		&session.ExpiresAt,
		&session.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.UserSession{}, models.ErrNotFound
		}

		return models.UserSession{}, fmt.Errorf("select user session: %w", err)
	}

	return session, nil
}

// Touch records that the session authenticated a request.
func (r *SessionRepository) Touch(ctx context.Context, id string, now time.Time) error {
	const query = `UPDATE user_sessions SET last_used_at = $2 WHERE id = $1::uuid`

	if _, err := queryWith(ctx, r.pool).Exec(ctx, query, id, now); err != nil {
		return fmt.Errorf("touch user session: %w", err)
	}

	return nil
}
