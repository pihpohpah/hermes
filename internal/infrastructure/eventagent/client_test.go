package eventagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientGetState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/state" {
			http.NotFound(w, r)

			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"version": 1,
			"pages": {},
			"events": {
				"abc123": {
					"event": {
						"title": "Спектакль",
						"category": "theater",
						"start_date": "2026-11-20",
						"start_time": "18:30",
						"venue": "Драмтеатр",
						"is_cancelled": false
					},
					"sources": ["https://afishagorodov.ru/vladivostok"],
					"first_seen": "2026-11-14T15:29:22+10:00",
					"last_seen": "2026-11-14T15:29:22+10:00"
				}
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})

	events, err := client.GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}

	event := events[0]
	if event.ExternalID == nil || *event.ExternalID != "abc123" {
		t.Errorf("ExternalID = %v, want abc123 from the map key", event.ExternalID)
	}
	if event.Venue != "Драмтеатр" {
		t.Errorf("Venue = %q", event.Venue)
	}
	if len(event.SourceURLs) != 1 {
		t.Errorf("SourceURLs = %v", event.SourceURLs)
	}
	if event.Status != "scheduled" {
		t.Errorf("Status = %q, want scheduled", event.Status)
	}
}

func TestClientGetStateServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})

	if _, err := client.GetState(context.Background()); err == nil {
		t.Fatal("expected error on 503")
	}
}
