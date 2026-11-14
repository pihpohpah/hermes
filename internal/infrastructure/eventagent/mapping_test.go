package eventagent

import (
	"testing"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

func TestEventResponseToDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		dto       eventResponse
		wantStart time.Time
		wantEnd   time.Time
		wantErr   bool
	}{
		{
			name: "с временем начала, без даты окончания",
			dto: eventResponse{
				ID:        "abc123",
				Title:     "Концерт",
				Category:  "concert",
				StartDate: "2026-11-20",
				StartTime: ptr("19:00"),
			},
			wantStart: time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 20, 13, 59, 59, 0, time.UTC),
		},
		{
			name: "без времени начала — старт в 00:00",
			dto: eventResponse{
				ID:        "abc124",
				Title:     "Выставка",
				Category:  "exhibition",
				StartDate: "2026-11-21",
			},
			wantStart: time.Date(2026, 11, 20, 14, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 21, 13, 59, 59, 0, time.UTC),
		},
		{
			name: "мультидневное событие",
			dto: eventResponse{
				ID:        "abc125",
				Title:     "Фестиваль",
				Category:  "festival",
				StartDate: "2026-11-22",
				EndDate:   ptr("2026-11-24"),
			},
			wantStart: time.Date(2026, 11, 21, 14, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 24, 13, 59, 59, 0, time.UTC),
		},
		{
			name: "импортная категория из миграции 002",
			dto: eventResponse{
				ID:        "abc126",
				Title:     "Спектакль",
				Category:  "theater",
				StartDate: "2026-11-23",
			},
			wantStart: time.Date(2026, 11, 22, 14, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 23, 13, 59, 59, 0, time.UTC),
		},
		{
			name: "неизвестная категория",
			dto: eventResponse{
				ID:        "abc127",
				Title:     "??",
				Category:  "nonsense",
				StartDate: "2026-11-23",
			},
			wantErr: true,
		},
		{
			name: "битая дата",
			dto: eventResponse{
				ID:        "abc128",
				Title:     "??",
				Category:  "concert",
				StartDate: "20.11.2026",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			event, err := tt.dto.toDomain()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", event)
				}

				return
			}
			if err != nil {
				t.Fatalf("toDomain: %v", err)
			}
			if !event.StartsAt.Equal(tt.wantStart) {
				t.Errorf("StartsAt = %s, want %s", event.StartsAt, tt.wantStart)
			}
			if !event.EndsAt.Equal(tt.wantEnd) {
				t.Errorf("EndsAt = %s, want %s", event.EndsAt, tt.wantEnd)
			}
			if event.ExternalID == nil || *event.ExternalID != tt.dto.ID {
				t.Errorf("ExternalID = %v, want %s", event.ExternalID, tt.dto.ID)
			}
			if _, err := models.NewEventCategoryCode(string(event.Category)); err != nil {
				t.Errorf("category %q not a valid domain code: %v", event.Category, err)
			}
		})
	}
}

func TestRunResponseToDomain(t *testing.T) {
	t.Parallel()

	dto := runResponse{
		RunID:  "run-1",
		Status: "completed",
		Events: []eventResponse{{
			ID: "e1", Title: "T", Category: "quiz", StartDate: "2026-11-25",
		}},
	}

	run, err := dto.toDomain()
	if err != nil {
		t.Fatalf("toDomain: %v", err)
	}
	if run.Status != models.RunStatusCompleted {
		t.Errorf("Status = %q, want completed", run.Status)
	}
	if run.Status.Terminal() != true {
		t.Error("completed status must be terminal")
	}
	if len(run.Events) != 1 {
		t.Fatalf("len(Events) = %d, want 1", len(run.Events))
	}
	if run.Events[0].Venue != "" {
		t.Errorf("empty venue must map to empty string, got %q", run.Events[0].Venue)
	}
}

func TestEventResponseToDomainExtras(t *testing.T) {
	t.Parallel()

	dto := eventResponse{
		ID:          "abc200",
		Title:       "Спектакль",
		Category:    "theater",
		StartDate:   "2026-11-25",
		Price:       ptr("от 300 ₽"),
		IsFree:      ptrBool(false),
		EventURL:    ptr("https://ticketland.ru/some-show"),
		IsCancelled: true,
		SourceURLs: []string{
			"https://vladivostok.ticketland.ru/spectacle",
			"https://vladivostok.ticketland.ru/other-page",
			"not a url",
			"https://visit-primorye.ru/events/123",
		},
	}

	event, err := dto.toDomain()
	if err != nil {
		t.Fatalf("toDomain: %v", err)
	}

	if event.Status != models.EventStatusCancelled {
		t.Errorf("Status = %q, want cancelled", event.Status)
	}
	if event.PriceFree == nil || *event.PriceFree {
		t.Errorf("PriceFree = %v, want false", event.PriceFree)
	}
	if event.PriceLabel != "от 300 ₽" {
		t.Errorf("PriceLabel = %q", event.PriceLabel)
	}
	if event.EventURL == nil || *event.EventURL != "https://ticketland.ru/some-show" {
		t.Errorf("EventURL = %v", event.EventURL)
	}
	wantSources := map[string]string{
		"vladivostok.ticketland.ru": "https://vladivostok.ticketland.ru/spectacle",
		"visit-primorye.ru":         "https://visit-primorye.ru/events/123",
	}
	if len(event.SourceURLs) != len(wantSources) {
		t.Fatalf("SourceURLs = %v, want %v", event.SourceURLs, wantSources)
	}
	for host, url := range wantSources {
		if event.SourceURLs[host] != url {
			t.Errorf("SourceURLs[%s] = %q, want %q", host, event.SourceURLs[host], url)
		}
	}

	// Не-http ссылка не должна уйти в БД: CHECK constraint её отверг бы.
	dto.EventURL = ptr("ftp://example.com")
	event, err = dto.toDomain()
	if err != nil {
		t.Fatalf("toDomain: %v", err)
	}
	if event.EventURL != nil {
		t.Errorf("non-http EventURL must be dropped, got %q", *event.EventURL)
	}
}

func ptr(s string) *string { return &s }

func ptrBool(b bool) *bool { return &b }

func TestStateResponseToDomainEvents(t *testing.T) {
	t.Parallel()

	dto := stateResponse{
		Events: map[string]stateEntryResponse{
			"deadbeef01": {
				Event: eventResponse{
					Title:     "Концерт из кеша",
					Category:  "concert",
					StartDate: "2026-11-20",
					StartTime: ptr("19:00"),
				},
				Sources: []string{
					"https://vladivostok.ticketland.ru/spectacle",
					"https://afishagorodov.ru/vladivostok/concerts",
				},
			},
			"deadbeef02": {
				Event: eventResponse{
					Title:     "Событие без источников",
					Category:  "quiz",
					StartDate: "2026-11-21",
				},
			},
		},
	}

	events, err := dto.toDomainEvents()
	if err != nil {
		t.Fatalf("toDomainEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}

	byID := make(map[string]models.Event, len(events))
	for _, event := range events {
		if event.ExternalID == nil {
			t.Fatal("external id must come from the state map key")
		}
		byID[*event.ExternalID] = event
	}

	cached := byID["deadbeef01"]
	if cached.Venue != "" || cached.Title != "Концерт из кеша" {
		t.Errorf("unexpected event: %+v", cached)
	}
	if len(cached.SourceURLs) != 2 {
		t.Fatalf("SourceURLs = %v, want 2 entries keyed by host", cached.SourceURLs)
	}
	if cached.SourceURLs["vladivostok.ticketland.ru"] == "" ||
		cached.SourceURLs["afishagorodov.ru"] == "" {
		t.Errorf("SourceURLs keyed by host: %v", cached.SourceURLs)
	}

	noSources := byID["deadbeef02"]
	if len(noSources.SourceURLs) != 0 {
		t.Errorf("SourceURLs = %v, want empty", noSources.SourceURLs)
	}
}

func TestStateResponseToDomainEventsBadEntry(t *testing.T) {
	t.Parallel()

	dto := stateResponse{
		Events: map[string]stateEntryResponse{
			"bad": {Event: eventResponse{Title: "?", Category: "nonsense", StartDate: "2026-11-20"}},
		},
	}

	if _, err := dto.toDomainEvents(); err == nil {
		t.Fatal("invalid entry must fail the mapping loudly")
	}
}
