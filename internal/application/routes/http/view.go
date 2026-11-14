package http

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

// Состояние экранов живёт в URL query — любая ссылка воспроизводит вид,
// кнопка «Назад» работает, а фрагменты для htmx строятся из тех же параметров.

const (
	scenarioCity   = "city"
	scenarioNature = "nature"
	scenarioSea    = "sea"
)

func validScenario(raw string) string {
	switch raw {
	case scenarioNature, scenarioSea:
		return raw
	default:
		return scenarioCity
	}
}

type mapQuery struct {
	Scenario string
	Q        string
	Category string
	Type     string // all | place | event
	Selected string // id объекта в превью
	Panel    string // route — редактор маршрута
	Tab      string // filters | list
	Filters  presenter.CatalogFilter
}

func parseMapQuery(r *http.Request) mapQuery {
	query := r.URL.Query()

	return mapQuery{
		Scenario: validScenario(query.Get("s")),
		Q:        strings.TrimSpace(query.Get("q")),
		Category: query.Get("cat"),
		Type:     parseType(query.Get("type")),
		Selected: query.Get("place"),
		Panel:    query.Get("panel"),
		Tab:      parseTab(query.Get("tab")),
		Filters:  parseCatalogFilter(query),
	}
}

func parseTab(raw string) string {
	if raw == "list" {
		return "list"
	}

	return "filters"
}

func parseType(raw string) string {
	if raw == "place" || raw == "event" {
		return raw
	}

	return "all"
}

type catalogQuery struct {
	Scenario   string
	Q          string
	Category   string
	Collection string
	Type       string
	Sort       string
	View       string
	Filters    presenter.CatalogFilter
}

func parseCatalogQuery(r *http.Request) catalogQuery {
	query := r.URL.Query()

	sortBy := query.Get("sort")
	if sortBy != "distance" && sortBy != "title" {
		sortBy = "recommended"
	}

	view := query.Get("view")
	if view != "list" {
		view = "grid"
	}

	return catalogQuery{
		Scenario:   validScenario(query.Get("s")),
		Q:        strings.TrimSpace(query.Get("q")),
		Category:   query.Get("cat"),
		Collection: query.Get("col"),
		Type:       parseType(query.Get("type")),
		Sort:       sortBy,
		View:       view,
		Filters:    parseCatalogFilter(query),
	}
}

// parseCatalogFilter читает расширенные фильтры сценария из query: free=1,
// walkable=1, kids=1, distance=30, difficulty=easy,hard, noCar=1, beach=sand,swim…
func parseCatalogFilter(query url.Values) presenter.CatalogFilter {
	filter := presenter.CatalogFilter{
		Free:     query.Get("free") == "1",
		Walkable: query.Get("walkable") == "1",
		Kids:     query.Get("kids") == "1",
		NoCar:    query.Get("noCar") == "1",
		Swim:     query.Get("swim") == "1",
	}

	if raw := query.Get("distance"); raw != "" {
		if km, err := strconv.ParseFloat(raw, 64); err == nil {
			filter.Distance = km
		}
	}
	filter.Difficulty = splitList(query.Get("difficulty"))
	filter.Beach = splitList(query.Get("beach"))

	return filter
}

func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}

	return out
}

func hasActiveFilters(f presenter.CatalogFilter) int {
	count := 0
	if f.Free {
		count++
	}
	if f.Walkable {
		count++
	}
	if f.Kids {
		count++
	}
	if f.Distance > 0 {
		count++
	}
	if f.NoCar {
		count++
	}
	if f.Swim {
		count++
	}
	count += len(f.Difficulty)
	count += len(f.Beach)

	return count
}

// catalogLoader собирает объединённую выдачу мест и событий для экранов.
type catalogLoader struct {
	places      ListPlacesUC
	events      ListEventsUC
	collections GetCollectionUC
	placesByID  placeGetter
}

// CatalogLoader собирает loader для Pages/Fragments из уже созданных use case.
func CatalogLoader(
	places ListPlacesUC,
	events ListEventsUC,
	collections GetCollectionUC,
	placesByID placeGetter,
) catalogLoader {
	return catalogLoader{
		places:      places,
		events:      events,
		collections: collections,
		placesByID:  placesByID,
	}
}

type placeGetter interface {
	GetManyByIDs(ctx context.Context, ids []string) ([]models.Place, error)
}

type catalogResult struct {
	Items          []presenter.CatalogItem
	TypeCounts     map[string]int
	CategoryCounts map[string]int
}

// loadItems выбирает и мерджит места и события по состоянию экрана.
func (l catalogLoader) loadItems(ctx context.Context, state catalogQuery) (catalogResult, error) {
	result := catalogResult{
		TypeCounts:     map[string]int{"all": 0, "place": 0, "event": 0},
		CategoryCounts: map[string]int{},
	}

	var items []presenter.CatalogItem

	if state.Type != "event" {
		filter := models.PlaceFilter{Limit: 500}
		layer := models.LayerCode(state.Scenario)
		filter.Layer = &layer
		if state.Q != "" {
			filter.Q = &state.Q
		}
		if state.Category != "" {
			category, err := models.NewCategoryCode(state.Category)
			if err != nil {
				return result, err
			}
			filter.Category = &category
		}

		places, err := l.places.Do(ctx, filter)
		if err != nil {
			return result, err
		}
		for _, place := range places {
			items = append(items, presenter.CatalogItemFromPlace(presenter.PlaceToHTTP(place)))
		}
	}

	if state.Type != "place" {
		filter := models.EventFilter{Q: state.Q, UpcomingOnly: true, Limit: 200}
		if state.Type == "event" && state.Category != "" {
			if category, err := models.NewEventCategoryCode(state.Category); err == nil {
				filter.Category = &category
			}
		}

		events, err := l.events.Do(ctx, filter)
		if err != nil {
			return result, err
		}
		placesByID, err := l.hydrateEventPlaces(ctx, events)
		if err != nil {
			return result, err
		}
		for _, event := range events {
			items = append(items, presenter.CatalogItemFromEvent(presenter.EventToHTTP(event), placesByID[derefString(event.PlaceID)]))
		}
	}

	items = l.applyCollection(ctx, items, state.Collection)

	filtered := make([]presenter.CatalogItem, 0, len(items))
	for _, item := range items {
		// Сценарий применяется к обоим типам: событие — городской объект
		// (scenario всегда "city"), поэтому в природе и на море оно отсекается.
		if item.Scenario != state.Scenario {
			continue
		}
		if !state.Filters.Matches(item) {
			continue
		}
		filtered = append(filtered, item)
	}

	presenter.SortCatalogItems(filtered, state.Sort)

	result.Items = filtered
	result.TypeCounts["place"] = countType(filtered, "place")
	result.TypeCounts["event"] = countType(filtered, "event")
	result.TypeCounts["all"] = len(filtered)
	for _, item := range filtered {
		if item.Category != "" {
			result.CategoryCounts[item.Category]++
		}
	}

	return result, nil
}

func countType(items []presenter.CatalogItem, itemType string) int {
	count := 0
	for _, item := range items {
		if item.Type == itemType {
			count++
		}
	}

	return count
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

// hydrateEventPlaces подтягивает места проведения событий одним запросом.
func (l catalogLoader) hydrateEventPlaces(ctx context.Context, events []models.Event) (map[string]*presenter.Place, error) {
	ids := make([]string, 0, len(events))
	seen := map[string]struct{}{}
	for _, event := range events {
		if event.PlaceID == nil {
			continue
		}
		if _, ok := seen[*event.PlaceID]; ok {
			continue
		}
		seen[*event.PlaceID] = struct{}{}
		ids = append(ids, *event.PlaceID)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	places, err := l.placesByID.GetManyByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make(map[string]*presenter.Place, len(places))
	for i := range places {
		item := presenter.PlaceToHTTP(places[i])
		out[item.ID] = &item
	}

	return out, nil
}

// applyCollection оставляет только объекты выбранной подборки.
func (l catalogLoader) applyCollection(ctx context.Context, items []presenter.CatalogItem, collectionID string) []presenter.CatalogItem {
	if collectionID == "" {
		return items
	}

	collection, err := l.collections.Do(ctx, collectionID)
	if err != nil {
		return items
	}

	allowed := make(map[string]struct{}, len(collection.Items))
	for _, ref := range collection.Items {
		allowed[ref.ItemType+":"+ref.ItemID] = struct{}{}
	}

	filtered := make([]presenter.CatalogItem, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.Type+":"+item.ID]; ok {
			filtered = append(filtered, item)
		}
	}

	return filtered
}
