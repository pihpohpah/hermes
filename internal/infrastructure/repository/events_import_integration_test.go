//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// TestUpsertImportedIntegration прогоняет реальный upsert через pgx против
// тестовой БД: запускать вручную с -tags integration и DATABASE_URL.
func TestUpsertImportedIntegration(t *testing.T) {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, "postgres://postgres:postgres@localhost:5432/hermes_mig_test?sslmode=disable")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	repo := NewEventRepository(pool)
	externalID := "int-test-1"
	title := "Интеграционный тест"

	base := models.Event{
		ExternalID: &externalID,
		Title:      title,
		Category:   models.EventCategoryCode("concert"),
		Status:     models.EventStatusScheduled,
		Venue:      "Приморская краевая филармония",
		StartsAt:   time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC),
		EndsAt:     time.Date(2026, 11, 20, 13, 59, 59, 0, time.UTC),
	}

	// 1. Вставка без координат (nil Location) — то самое падение 42P08.
	inserted, err := repo.UpsertImported(ctx, base)
	if err != nil {
		t.Fatalf("upsert without location: %v", err)
	}
	if !inserted {
		t.Error("first upsert must insert")
	}

	// 2. Конфликт без координат: старое значение location не трогаем, оно NULL.
	inserted, err = repo.UpsertImported(ctx, base)
	if err != nil {
		t.Fatalf("conflict without location: %v", err)
	}
	if inserted {
		t.Error("second upsert must update")
	}

	// 3. Вставка с координатами.
	point, err := models.NewPoint(43.1155, 131.8855)
	if err != nil {
		t.Fatalf("point: %v", err)
	}
	located := base
	located.ExternalID = strPtr("int-test-2")
	located.Location = &point
	if _, err := repo.UpsertImported(ctx, located); err != nil {
		t.Fatalf("upsert with location: %v", err)
	}

	// 4. Конфликт без координат: location сохраняется (COALESCE).
	located.ExternalID = strPtr("int-test-2")
	located.Location = nil
	if _, err := repo.UpsertImported(ctx, located); err != nil {
		t.Fatalf("conflict keeps location: %v", err)
	}

	// 5. Смотрим результат напрямую: у строки int-test-2 location должен
	// остаться после конфликтного обновления без координат.
	var (
		storedTitle string
		storedLon   *float64
	)
	err = pool.QueryRow(
		ctx, "SELECT title, ST_X(location::geometry) FROM events WHERE external_id = 'int-test-2'",
	).Scan(&storedTitle, &storedLon)
	if err != nil {
		t.Fatalf("lookup by external id: %v", err)
	}
	if storedLon == nil {
		t.Error("location must survive conflict update without coordinates")
	}
	if storedTitle != title {
		t.Errorf("title = %q, want %q", storedTitle, title)
	}

	// 6. У строки int-test-1 (вставка без координат) location NULL — ок.
	var nilLon *float64
	err = pool.QueryRow(
		ctx, "SELECT ST_X(location::geometry) FROM events WHERE external_id = 'int-test-1'",
	).Scan(&nilLon)
	if err != nil {
		t.Fatalf("lookup int-test-1: %v", err)
	}
	if nilLon != nil {
		t.Errorf("int-test-1 location = %v, want NULL", *nilLon)
	}
}

func strPtr(s string) *string { return &s }
