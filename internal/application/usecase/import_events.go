package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// ImportEventsUseCase прогоняет импорт событий из event-agent: запускает прогон
// скрапера, дожидается завершения, прогеокодирует площадки и кладёт события в
// каталог по стабильному external_id.
type ImportEventsUseCase struct {
	agent        eventAgentClient
	geocoder     venueGeocoder
	events       importedEventStore
	pollInterval time.Duration
	maxWait      time.Duration
	sleep        func(ctx context.Context, d time.Duration) error
}

type eventAgentClient interface {
	StartRun(ctx context.Context) (models.ImportedRun, error)
	GetRun(ctx context.Context, runID string) (models.ImportedRun, error)
}

type venueGeocoder interface {
	Geocode(ctx context.Context, address string) (models.Point, error)
}

type importedEventStore interface {
	UpsertImported(ctx context.Context, event models.Event) (bool, error)
}

func NewImportEventsUseCase(
	agent eventAgentClient,
	geocoder venueGeocoder,
	events importedEventStore,
	pollInterval time.Duration,
	maxWait time.Duration,
) *ImportEventsUseCase {
	if pollInterval <= 0 {
		pollInterval = 10 * time.Second
	}
	if maxWait <= 0 {
		maxWait = 25 * time.Minute
	}

	return &ImportEventsUseCase{
		agent:        agent,
		geocoder:     geocoder,
		events:       events,
		pollInterval: pollInterval,
		maxWait:      maxWait,
		sleep:        sleepCtx,
	}
}

// ImportReport — итог одного импорта для лога команды.
type ImportReport struct {
	RunID         string
	Status        models.RunStatus
	StartedAt     time.Time
	FinishedAt    time.Time
	Created       int
	Updated       int
	Geocoded      int
	NoVenue       int
	GeocodeFailed int
}

func (u *ImportEventsUseCase) Do(ctx context.Context) (ImportReport, error) {
	report := ImportReport{StartedAt: time.Now().UTC()}

	run, err := u.agent.StartRun(ctx)
	if err != nil {
		return report, fmt.Errorf("start agent run: %w", err)
	}
	report.RunID = run.ID

	waitCtx, cancel := context.WithTimeout(ctx, u.maxWait)
	defer cancel()

	for !run.Status.Terminal() {
		if err := u.sleep(waitCtx, u.pollInterval); err != nil {
			return report, fmt.Errorf("wait for run `%s`: %w", run.ID, err)
		}

		run, err = u.agent.GetRun(waitCtx, run.ID)
		if err != nil {
			return report, fmt.Errorf("poll run `%s`: %w", run.ID, err)
		}
	}

	report.Status = run.Status
	if run.Status == models.RunStatusFailed {
		detail := "unknown error"
		if run.Error != nil && *run.Error != "" {
			detail = *run.Error
		}

		return report, fmt.Errorf("agent run `%s` failed: %s", run.ID, detail)
	}

	cache := make(map[string]*models.Point)
	for i := range run.Events {
		event := run.Events[i]
		event.Location = u.resolveLocation(waitCtx, event, cache)

		inserted, err := u.events.UpsertImported(ctx, event)
		if err != nil {
			return report, fmt.Errorf("store event `%s`: %w", eventTitle(event), err)
		}
		if inserted {
			report.Created++
		} else {
			report.Updated++
		}
	}

	report.Geocoded = countGeocoded(cache)
	report.GeocodeFailed = countFailed(cache)
	report.NoVenue = countNoVenue(run.Events)
	report.FinishedAt = time.Now().UTC()

	return report, nil
}

// resolveLocation отдаёт координаты площадки: из кэша прогона или из геокодера.
// Ошибки геокодинга и пустой venue — не повод прерывать импорт: событие просто
// остаётся без координат.
func (u *ImportEventsUseCase) resolveLocation(
	ctx context.Context,
	event models.Event,
	cache map[string]*models.Point,
) *models.Point {
	if event.Venue == "" {
		return nil
	}

	if point, ok := cache[event.Venue]; ok {
		return point
	}

	point, err := u.geocoder.Geocode(ctx, event.Venue)
	if err != nil {
		cache[event.Venue] = nil

		return nil
	}
	cache[event.Venue] = &point

	return &point
}

func eventTitle(event models.Event) string {
	if event.ExternalID != nil {
		return *event.ExternalID
	}

	return event.Title
}

func countGeocoded(cache map[string]*models.Point) int {
	count := 0
	for _, point := range cache {
		if point != nil {
			count++
		}
	}

	return count
}

func countFailed(cache map[string]*models.Point) int {
	count := 0
	for _, point := range cache {
		if point == nil {
			count++
		}
	}

	return count
}

func countNoVenue(events []models.Event) int {
	count := 0
	for _, event := range events {
		if event.Venue == "" {
			count++
		}
	}

	return count
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
