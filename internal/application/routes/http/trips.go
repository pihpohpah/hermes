package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

type ListTripsUC interface {
	Do(ctx context.Context, filter models.TripFilter) ([]models.TripWithConditions, error)
}

type ListTripsHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[[]presenter.Trip]
	useCase        ListTripsUC
}

func NewListTripsHandler(logger *slog.Logger, uc ListTripsUC) *ListTripsHandler {
	return &ListTripsHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[[]presenter.Trip](logger),
		useCase:        uc,
	}
}

func (h *ListTripsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	// The owner comes from the session, never from the query string: listing
	// always returns the caller's own trips.
	auth, ok := AuthFromContext(r.Context())
	if !ok {
		h.responseWriter.Error(w, http.StatusUnauthorized, models.ErrUnauthorized)

		return
	}

	filter := models.TripFilter{UserID: auth.User.ID}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			h.responseWriter.Error(w, http.StatusBadRequest, models.Invalid("`limit` must be an integer"))

			return
		}

		filter.Limit = parsed
	}

	trips, err := h.useCase.Do(r.Context(), filter)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.TripsWithConditionsToHTTP(trips))
}

type GetTripUC interface {
	Do(ctx context.Context, id string, userID string) (models.TripWithConditions, error)
}

type GetTripHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.Trip]
	useCase        GetTripUC
}

func NewGetTripHandler(logger *slog.Logger, uc GetTripUC) *GetTripHandler {
	return &GetTripHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.Trip](logger),
		useCase:        uc,
	}
}

func (h *GetTripHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	auth, ok := AuthFromContext(r.Context())
	if !ok {
		h.responseWriter.Error(w, http.StatusUnauthorized, models.ErrUnauthorized)

		return
	}

	trip, err := h.useCase.Do(r.Context(), r.PathValue("id"), auth.User.ID)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.TripWithConditionsToHTTP(trip))
}

type CreateTripUC interface {
	Do(ctx context.Context, in models.CreateTripInput) (models.Trip, error)
}

type CreateTripHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.Trip]
	useCase        CreateTripUC
}

func NewCreateTripHandler(logger *slog.Logger, uc CreateTripUC) *CreateTripHandler {
	return &CreateTripHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.Trip](logger),
		useCase:        uc,
	}
}

type createTripStopRequest struct {
	PlaceID  string `json:"place_id"`
	ArriveAt string `json:"arrive_at"`
}

type createTripRequest struct {
	PlannedAt   string                  `json:"planned_at"`
	StopMinutes int                     `json:"stop_minutes"`
	Activity    string                  `json:"activity"`
	Notes       string                  `json:"notes"`
	Stops       []createTripStopRequest `json:"stops"`
}

func (h *CreateTripHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method POST expected"))

		return
	}

	var body createTripRequest
	if err := decodeJSON(r, &body); err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	plannedAt, err := time.Parse(time.RFC3339, body.PlannedAt)
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest,
			models.Invalid("`planned_at` must be an RFC3339 timestamp"))

		return
	}

	activity, err := models.NewActivity(body.Activity)
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	stops := make([]models.TripStop, 0, len(body.Stops))
	for _, item := range body.Stops {
		arriveAt, err := time.Parse(time.RFC3339, item.ArriveAt)
		if err != nil {
			h.responseWriter.Error(w, http.StatusBadRequest,
				models.Invalid("`arrive_at` must be an RFC3339 timestamp"))

			return
		}

		stops = append(stops, models.TripStop{
			PlaceID:  item.PlaceID,
			Position: len(stops),
			ArriveAt: arriveAt,
		})
	}

	auth, ok := AuthFromContext(r.Context())
	if !ok {
		h.responseWriter.Error(w, http.StatusUnauthorized, models.ErrUnauthorized)

		return
	}

	trip, err := h.useCase.Do(r.Context(), models.CreateTripInput{
		UserID:      auth.User.ID,
		PlannedAt:   plannedAt,
		StopMinutes: body.StopMinutes,
		Activity:    activity,
		Notes:       body.Notes,
		Stops:       stops,
	})
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.SuccessWithStatus(w, http.StatusCreated, presenter.TripToHTTP(trip))
}

type UpdateTripUC interface {
	Do(ctx context.Context, in models.UpdateTripInput) (models.Trip, error)
}

type UpdateTripHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.Trip]
	useCase        UpdateTripUC
}

func NewUpdateTripHandler(logger *slog.Logger, uc UpdateTripUC) *UpdateTripHandler {
	return &UpdateTripHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.Trip](logger),
		useCase:        uc,
	}
}

type updateTripRequest struct {
	Status string  `json:"status"`
	Notes  *string `json:"notes"`
}

func (h *UpdateTripHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method PATCH expected"))

		return
	}

	var body updateTripRequest
	if err := decodeJSON(r, &body); err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	auth, ok := AuthFromContext(r.Context())
	if !ok {
		h.responseWriter.Error(w, http.StatusUnauthorized, models.ErrUnauthorized)

		return
	}

	input := models.UpdateTripInput{
		ID:     r.PathValue("id"),
		UserID: auth.User.ID,
		Notes:  body.Notes,
	}

	if body.Status != "" {
		status, err := models.NewTripStatus(body.Status)
		if err != nil {
			h.responseWriter.Error(w, http.StatusBadRequest, err)

			return
		}

		input.Status = &status
	}

	if input.Status == nil && input.Notes == nil {
		h.responseWriter.Error(w, http.StatusBadRequest,
			models.Invalid("nothing to update: provide `status` or `notes`"))

		return
	}

	trip, err := h.useCase.Do(r.Context(), input)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.TripToHTTP(trip))
}
