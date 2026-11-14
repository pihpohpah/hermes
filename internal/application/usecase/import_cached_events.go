package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// eventStateReader читает кеш event-agent (GET /state) — все известные агенту
// события без запуска скрапинга.
type eventStateReader interface {
	GetState(ctx context.Context) ([]models.Event, error)
}

// ImportCachedEventsUseCase пересобирает каталог целиком из кеша агента:
// в отличие от ImportEventsUseCase не запускает прогон и не ждёт его, а берёт
// /state как есть. Полезно для первичного наполнения и для догона событий,
// которые прогоны уже отдали, но импортёр по какой-то причине не забрал.
type ImportCachedEventsUseCase struct {
	eventImportSink
	states eventStateReader
}

func NewImportCachedEventsUseCase(
	states eventStateReader,
	geocoder venueGeocoder,
	events importedEventStore,
) *ImportCachedEventsUseCase {
	return &ImportCachedEventsUseCase{
		eventImportSink: newEventImportSink(geocoder, events),
		states:          states,
	}
}

func (u *ImportCachedEventsUseCase) Do(ctx context.Context) (ImportReport, error) {
	report := ImportReport{StartedAt: time.Now().UTC()}

	batch, err := u.states.GetState(ctx)
	if err != nil {
		return report, fmt.Errorf("load agent state: %w", err)
	}

	if err := u.importEvents(ctx, batch, &report); err != nil {
		return report, err
	}
	report.FinishedAt = time.Now().UTC()

	return report, nil
}
