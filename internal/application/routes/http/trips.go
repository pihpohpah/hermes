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
	Do(ctx context.Context, filter models.TripFilter) ([]models.Trip, error)
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

	query := r.URL.Query()

	filter := models.TripFilter{UserID: query.Get("user_id")}
	if raw := query.Get("limit"); raw != "" {
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

	h.responseWriter.Success(w, presenter.TripsToHTTP(trips))
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

type createTripRequest struct {
	UserID    string `json:"user_id"`
	PlaceID   string `json:"place_id"`
	PlannedAt string `json:"planned_at"`
	Activity  string `json:"activity"`
	Notes     string `json:"notes"`
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

	trip, err := h.useCase.Do(r.Context(), models.CreateTripInput{
		UserID:    body.UserID,
		PlaceID:   body.PlaceID,
		PlannedAt: plannedAt,
		Activity:  activity,
		Notes:     body.Notes,
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

	input := models.UpdateTripInput{
		ID:    r.PathValue("id"),
		Notes: body.Notes,
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
