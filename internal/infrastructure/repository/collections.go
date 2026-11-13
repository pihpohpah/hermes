package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// CollectionRepository stores editorial catalog collections: ordered lists
// of places and events.
type CollectionRepository struct {
	pool *pgxpool.Pool
}

func NewCollectionRepository(pool *pgxpool.Pool) *CollectionRepository {
	return &CollectionRepository{pool: pool}
}

func (r *CollectionRepository) List(ctx context.Context, scenario *models.LayerCode) ([]models.Collection, error) {
	var (
		where string
		args  []any
	)

	if scenario != nil {
		where = "WHERE scenario = $1 OR scenario IS NULL"
		args = append(args, string(*scenario))
	}

	query := fmt.Sprintf(`
		SELECT id::text, slug, title, scenario, cover
		FROM collections
		%s
		ORDER BY title ASC
	`, where)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query collections: %w", err)
	}
	defer rows.Close()

	result := make([]models.Collection, 0)
	for rows.Next() {
		var (
			collection  models.Collection
			scenarioRaw *string
		)
		if err := rows.Scan(
			&collection.ID,
			&collection.Slug,
			&collection.Title,
			&scenarioRaw,
			&collection.Cover,
		); err != nil {
			return nil, fmt.Errorf("scan collection row: %w", err)
		}
		if scenarioRaw != nil {
			layer, err := models.NewLayerCode(*scenarioRaw)
			if err != nil {
				return nil, fmt.Errorf("parse collection scenario: %w", err)
			}
			collection.Scenario = &layer
		}
		result = append(result, collection)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate collections rows: %w", err)
	}

	return result, nil
}

// GetWithItems loads one collection by UUID or slug together with its
// ordered item references.
func (r *CollectionRepository) GetWithItems(ctx context.Context, id string) (models.Collection, error) {
	const query = `
		SELECT id::text, slug, title, scenario, cover
		FROM collections
		WHERE id = $1::uuid OR slug = $1
	`

	var (
		collection  models.Collection
		scenarioRaw *string
	)

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&collection.ID,
		&collection.Slug,
		&collection.Title,
		&scenarioRaw,
		&collection.Cover,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Collection{}, models.ErrNotFound
		}

		return models.Collection{}, fmt.Errorf("query collection by id: %w", err)
	}
	if scenarioRaw != nil {
		layer, err := models.NewLayerCode(*scenarioRaw)
		if err != nil {
			return models.Collection{}, fmt.Errorf("parse collection scenario: %w", err)
		}
		collection.Scenario = &layer
	}

	const itemsQuery = `
		SELECT item_type, item_id::text, position
		FROM collection_items
		WHERE collection_id = $1::uuid
		ORDER BY position ASC, item_type ASC, item_id ASC
	`

	rows, err := r.pool.Query(ctx, itemsQuery, collection.ID)
	if err != nil {
		return models.Collection{}, fmt.Errorf("query collection items: %w", err)
	}
	defer rows.Close()

	items := make([]models.CollectionItemRef, 0)
	for rows.Next() {
		var item models.CollectionItemRef
		if err := rows.Scan(&item.ItemType, &item.ItemID, &item.Position); err != nil {
			return models.Collection{}, fmt.Errorf("scan collection item row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return models.Collection{}, fmt.Errorf("iterate collection items rows: %w", err)
	}

	collection.Items = items

	return collection, nil
}
