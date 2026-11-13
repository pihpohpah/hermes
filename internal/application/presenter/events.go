package presenter

import (
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// Event is the HTTP view of a catalog event.
type Event struct {
	ID          string `json:"id"`
	Slug        string `json:"slug,omitempty"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Short       string `json:"short"`
	Description string `json:"description"`
	PlaceID     string `json:"place_id,omitempty"`
	PlaceName   string `json:"place_name,omitempty"`
	Venue       string `json:"venue"`
	Organizer   string `json:"organizer"`
	Age         string `json:"age"`
	StartsAt    string `json:"starts_at"`
	EndsAt      string `json:"ends_at"`
	CreatedAt   string `json:"created_at"`
}

func EventToHTTP(in models.Event) Event {
	out := Event{
		ID:          in.ID,
		Title:       in.Title,
		Category:    string(in.Category),
		Short:       in.Short,
		Description: in.Description,
		Venue:       in.Venue,
		Organizer:   in.Organizer,
		Age:         in.Age,
		StartsAt:    in.StartsAt.UTC().Format(time.RFC3339),
		EndsAt:      in.EndsAt.UTC().Format(time.RFC3339),
		CreatedAt:   in.CreatedAt.UTC().Format(time.RFC3339),
	}
	if in.Slug != nil {
		out.Slug = *in.Slug
	}
	if in.PlaceID != nil {
		out.PlaceID = *in.PlaceID
	}
	if in.PlaceName != nil {
		out.PlaceName = *in.PlaceName
	}

	return out
}

func EventsToHTTP(in []models.Event) []Event {
	out := make([]Event, len(in))
	for i := range in {
		out[i] = EventToHTTP(in[i])
	}

	return out
}

// Collection is a list entry without hydrated items.
type Collection struct {
	ID       string `json:"id"`
	Slug     string `json:"slug,omitempty"`
	Title    string `json:"title"`
	Scenario string `json:"scenario,omitempty"`
	Cover    string `json:"cover"`
}

// CollectionItemSummary is a hydrated catalog entry inside a collection.
type CollectionItemSummary struct {
	Type     string  `json:"type"`
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Category string  `json:"category"`
	Lat      float64 `json:"lat,omitempty"`
	Lon      float64 `json:"lon,omitempty"`
	StartsAt string  `json:"starts_at,omitempty"`
}

// CollectionDetail is a collection with its ordered item summaries.
type CollectionDetail struct {
	ID       string                  `json:"id"`
	Slug     string                  `json:"slug,omitempty"`
	Title    string                  `json:"title"`
	Scenario string                  `json:"scenario,omitempty"`
	Cover    string                  `json:"cover"`
	Items    []CollectionItemSummary `json:"items"`
}

func CollectionToHTTP(in models.Collection) Collection {
	out := Collection{
		ID:    in.ID,
		Title: in.Title,
		Cover: in.Cover,
	}
	if in.Slug != nil {
		out.Slug = *in.Slug
	}
	if in.Scenario != nil {
		out.Scenario = string(*in.Scenario)
	}

	return out
}

func CollectionsToHTTP(in []models.Collection) []Collection {
	out := make([]Collection, len(in))
	for i := range in {
		out[i] = CollectionToHTTP(in[i])
	}

	return out
}

// CollectionDetailToHTTP hydrates the collection item references with the
// loaded place and event rows, keeping the collection's own ordering.
func CollectionDetailToHTTP(
	in models.Collection,
	places []models.Place,
	events []models.Event,
) CollectionDetail {
	byID := make(map[string]CollectionItemSummary, len(places)+len(events))

	for _, place := range places {
		summary := CollectionItemSummary{
			Type:  "place",
			ID:    place.ID,
			Title: place.Name,
			Lat:   place.Location.Lat,
			Lon:   place.Location.Lon,
		}
		if place.Category != nil {
			summary.Category = string(*place.Category)
		}
		byID[place.ID] = summary
	}

	for _, event := range events {
		summary := CollectionItemSummary{
			Type:     "event",
			ID:       event.ID,
			Title:    event.Title,
			Category: string(event.Category),
			StartsAt: event.StartsAt.UTC().Format(time.RFC3339),
		}
		byID[event.ID] = summary
	}

	items := make([]CollectionItemSummary, 0, len(in.Items))
	for _, ref := range in.Items {
		if summary, ok := byID[ref.ItemID]; ok {
			items = append(items, summary)
		}
	}

	out := CollectionDetail{
		ID:    in.ID,
		Title: in.Title,
		Cover: in.Cover,
		Items: items,
	}
	if in.Slug != nil {
		out.Slug = *in.Slug
	}
	if in.Scenario != nil {
		out.Scenario = string(*in.Scenario)
	}

	return out
}

// PlaceStats is the catalog aggregate for the directory tabs.
type PlaceStats struct {
	Places         int            `json:"places"`
	Events         int            `json:"events"`
	TypeCounts     map[string]int `json:"type_counts"`
	CategoryCounts map[string]int `json:"category_counts"`
}

func PlaceStatsToHTTP(in models.PlaceStats) PlaceStats {
	return PlaceStats{
		Places: in.Places,
		Events: in.Events,
		TypeCounts: map[string]int{
			"place": in.Places,
			"event": in.Events,
		},
		CategoryCounts: in.CategoryCounts,
	}
}
