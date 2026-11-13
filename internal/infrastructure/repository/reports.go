package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

const defaultReportLimit = 50

type ReportRepository struct {
	pool *pgxpool.Pool
}

func NewReportRepository(pool *pgxpool.Pool) *ReportRepository {
	return &ReportRepository{pool: pool}
}

func (r *ReportRepository) ListByPlace(ctx context.Context, filter models.ReportFilter) ([]models.Report, error) {
	if filter.PlaceID == "" {
		return nil, models.Invalid("`place_id` is required")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultReportLimit
	}

	const query = `
		SELECT id::text, place_id::text, user_id::text, type, comment, created_at
		FROM reports
		WHERE place_id = $1::uuid
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, filter.PlaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("query reports: %w", err)
	}
	defer rows.Close()

	result := make([]models.Report, 0)
	for rows.Next() {
		report, scanErr := scanReport(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reports rows: %w", err)
	}

	return result, nil
}

func (r *ReportRepository) Create(ctx context.Context, in models.CreateReportInput) (models.Report, error) {
	const query = `
		INSERT INTO reports (place_id, user_id, type, comment)
		VALUES ($1::uuid, $2::uuid, $3, $4)
		RETURNING id::text, place_id::text, user_id::text, type, comment, created_at
	`

	row := r.pool.QueryRow(ctx, query, in.PlaceID, in.UserID, string(in.Type), in.Comment)

	report, err := scanReport(row)
	if err != nil {
		return models.Report{}, fmt.Errorf("create report: %w", err)
	}

	return report, nil
}

func scanReport(row rowScanner) (models.Report, error) {
	var (
		report    models.Report
		rawType   string
		createdAt time.Time
	)

	if err := row.Scan(
		&report.ID,
		&report.PlaceID,
		&report.UserID,
		&rawType,
		&report.Comment,
		&createdAt,
	); err != nil {
		return models.Report{}, err
	}

	parsedType, err := models.NewReportType(rawType)
	if err != nil {
		return models.Report{}, fmt.Errorf("parse report type `%s`: %w", rawType, err)
	}

	report.Type = parsedType
	report.CreatedAt = createdAt

	return report, nil
}
