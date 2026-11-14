package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create persists a new user; the database generates the UUID, so the input
// profile arrives without an ID and leaves with one.
func (r *UserRepository) Create(ctx context.Context, user models.User) (models.User, error) {
	const query = `
		INSERT INTO users (display_name, timezone)
		VALUES ($1, $2)
		RETURNING id::text, display_name, timezone, created_at, updated_at
	`

	var created models.User

	err := queryWith(ctx, r.pool).QueryRow(ctx, query, user.DisplayName, user.Timezone).Scan(
		&created.ID,
		&created.DisplayName,
		&created.Timezone,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return models.User{}, fmt.Errorf("insert user: %w", err)
	}

	return created, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (models.User, error) {
	const query = `
		SELECT id::text, display_name, timezone, created_at, updated_at
		FROM users
		WHERE id = $1::uuid
	`

	var user models.User

	err := queryWith(ctx, r.pool).QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.DisplayName,
		&user.Timezone,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.User{}, models.ErrNotFound
		}

		return models.User{}, fmt.Errorf("select user: %w", err)
	}

	return user, nil
}

// UpdateDisplayName меняет отображаемое имя профиля.
func (r *UserRepository) UpdateDisplayName(ctx context.Context, id, displayName string) (models.User, error) {
	const query = `
		UPDATE users
		SET display_name = $2, updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, display_name, timezone, created_at, updated_at
	`

	var updated models.User

	err := queryWith(ctx, r.pool).QueryRow(ctx, query, id, displayName).Scan(
		&updated.ID,
		&updated.DisplayName,
		&updated.Timezone,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.User{}, models.ErrNotFound
		}

		return models.User{}, fmt.Errorf("update user display name: %w", err)
	}

	return updated, nil
}
