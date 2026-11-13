package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

type ListEventsUC interface {
	Do(ctx context.Context, filter models.EventFilter) ([]models.Event, error)
}

type ListEventsHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[[]presenter.Event]
	useCase        ListEventsUC
}

func NewListEventsHandler(logger *slog.Logger, uc ListEventsUC) *ListEventsHandler {
	return &ListEventsHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[[]presenter.Event](logger),
		useCase:        uc,
	}
}

// GET /events?place_id=&category=&q=&all=1 — upcoming events by default;
// ?all=1 includes past ones.
func (h *ListEventsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	filter, err := parseEventFilter(r)
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	events, err := h.useCase.Do(r.Context(), filter)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.EventsToHTTP(events))
}

func parseEventFilter(r *http.Request) (models.EventFilter, error) {
	filter := models.EventFilter{
		UpcomingOnly: r.URL.Query().Get("all") != "1",
	}
	query := r.URL.Query()

	if raw := query.Get("place_id"); raw != "" {
		placeID, err := models.ValidateUUID("place_id", raw)
		if err != nil {
			return filter, err
		}
		filter.PlaceID = &placeID
	}

	if raw := query.Get("category"); raw != "" {
		category, err := models.NewEventCategoryCode(raw)
		if err != nil {
			return filter, err
		}
		filter.Category = &category
	}

	filter.Q = query.Get("q")

	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			return filter, models.Invalid("`limit` must be a positive integer")
		}
		filter.Limit = limit
	}

	return filter, nil
}

type GetEventUC interface {
	Do(ctx context.Context, id string) (models.Event, error)
}

type GetEventHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.Event]
	useCase        GetEventUC
}

func NewGetEventHandler(logger *slog.Logger, uc GetEventUC) *GetEventHandler {
	return &GetEventHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.Event](logger),
		useCase:        uc,
	}
}

func (h *GetEventHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	event, err := h.useCase.Do(r.Context(), r.PathValue("id"))
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.EventToHTTP(event))
}

type ListCollectionsUC interface {
	Do(ctx context.Context, scenario *models.LayerCode) ([]models.Collection, error)
}

type ListCollectionsHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[[]presenter.Collection]
	useCase        ListCollectionsUC
}

func NewListCollectionsHandler(logger *slog.Logger, uc ListCollectionsUC) *ListCollectionsHandler {
	return &ListCollectionsHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[[]presenter.Collection](logger),
		useCase:        uc,
	}
}

// GET /collections?scenario=city — scenario-less collections apply to all.
func (h *ListCollectionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	var scenario *models.LayerCode
	if raw := r.URL.Query().Get("scenario"); raw != "" {
		layer, err := models.NewLayerCode(raw)
		if err != nil {
			h.responseWriter.Error(w, http.StatusBadRequest, err)

			return
		}
		scenario = &layer
	}

	collections, err := h.useCase.Do(r.Context(), scenario)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.CollectionsToHTTP(collections))
}

type GetCollectionUC interface {
	Do(ctx context.Context, id string) (models.Collection, error)
}

type GetCollectionHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.CollectionDetail]
	useCase        GetCollectionUC
	places         placeSummaries
	events         eventSummaries
}

type placeSummaries interface {
	GetManyByIDs(ctx context.Context, ids []string) ([]models.Place, error)
}

type eventSummaries interface {
	GetManyByIDs(ctx context.Context, ids []string) ([]models.Event, error)
}

func NewGetCollectionHandler(
	logger *slog.Logger,
	uc GetCollectionUC,
	places placeSummaries,
	events eventSummaries,
) *GetCollectionHandler {
	return &GetCollectionHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.CollectionDetail](logger),
		useCase:        uc,
		places:         places,
		events:         events,
	}
}

// GET /collections/{id} — the collection with its items hydrated to
// lightweight summaries (type, id, title, category, coordinates).
func (h *GetCollectionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	collection, err := h.useCase.Do(r.Context(), r.PathValue("id"))
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	placeIDs, eventIDs := splitItemIDs(collection.Items)

	places, err := h.places.GetManyByIDs(r.Context(), placeIDs)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	events, err := h.events.GetManyByIDs(r.Context(), eventIDs)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.CollectionDetailToHTTP(collection, places, events))
}

func splitItemIDs(items []models.CollectionItemRef) (places, events []string) {
	for _, item := range items {
		switch item.ItemType {
		case "event":
			events = append(events, item.ItemID)
		default:
			places = append(places, item.ItemID)
		}
	}

	return places, events
}
