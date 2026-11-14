package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// isUniqueViolation reports a PostgreSQL unique-constraint violation (23505),
// whatever the concrete pgx error wrapping happens to be.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError

	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// IdentityRepository хранит внешние способы входа (Telegram, будущие OAuth) в
// user_identities. Пара (provider, subject) уникальна на уровне БД, поэтому
// гонка двух одновременных первых входов с одним tgId завершается одной
// записью: проигравший получит ошибку уникальности и перечитает результат.
type IdentityRepository struct {
	pool *pgxpool.Pool
}

func NewIdentityRepository(pool *pgxpool.Pool) *IdentityRepository {
	return &IdentityRepository{pool: pool}
}

// GetByProviderSubject находит привязку по паре (provider, subject).
func (r *IdentityRepository) GetByProviderSubject(
	ctx context.Context,
	provider, subject string,
) (models.UserIdentity, error) {
	const query = `
		SELECT id::text, user_id::text, provider, subject, data, created_at
		FROM user_identities
		WHERE provider = $1 AND subject = $2
	`

	var identity models.UserIdentity

	err := queryWith(ctx, r.pool).QueryRow(ctx, query, provider, subject).Scan(
		&identity.ID,
		&identity.UserID,
		&identity.Provider,
		&identity.Subject,
		&identity.Data,
		&identity.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.UserIdentity{}, models.ErrNotFound
		}

		return models.UserIdentity{}, fmt.Errorf("select identity: %w", err)
	}

	return identity, nil
}

// Create привязывает provider-аккаунт к существующему пользователю. Конфликт
// по (provider, subject) — это models.ErrConflict, а не падение транзакции.
func (r *IdentityRepository) Create(ctx context.Context, identity models.UserIdentity) (models.UserIdentity, error) {
	const query = `
		INSERT INTO user_identities (user_id, provider, subject, data)
		VALUES ($1::uuid, $2, $3, $4)
		RETURNING id::text, user_id::text, provider, subject, data, created_at
	`

	var created models.UserIdentity

	err := queryWith(ctx, r.pool).QueryRow(ctx, query,
		identity.UserID, identity.Provider, identity.Subject, identity.Data,
	).Scan(
		&created.ID,
		&created.UserID,
		&created.Provider,
		&created.Subject,
		&created.Data,
		&created.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return models.UserIdentity{}, fmt.Errorf(
				"identity (%s, %s): %w", identity.Provider, identity.Subject, models.ErrConflict,
			)
		}

		return models.UserIdentity{}, fmt.Errorf("insert identity: %w", err)
	}

	return created, nil
}
