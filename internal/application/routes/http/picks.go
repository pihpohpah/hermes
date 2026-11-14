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

type GetForecastPicksUC interface {
	Do(ctx context.Context, in models.PickQuery) ([]models.ConditionPick, error)
}

// GetForecastPicksHandler отдаёт ранжированные места для одного дня и
// активности: «куда поехать по погоде». GET /forecast/picks?activity=&date=.
type GetForecastPicksHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[[]presenter.ConditionPickDTO]
	useCase        GetForecastPicksUC
}

func NewGetForecastPicksHandler(logger *slog.Logger, uc GetForecastPicksUC) *GetForecastPicksHandler {
	return &GetForecastPicksHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[[]presenter.ConditionPickDTO](logger),
		useCase:        uc,
	}
}

func (h *GetForecastPicksHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	date, err := parseTargetDate(r.URL.Query().Get("date"))
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	query := models.PickQuery{Date: date}
	if activityRaw := r.URL.Query().Get("activity"); activityRaw != "" {
		activity, aErr := models.NewActivity(activityRaw)
		if aErr != nil {
			h.responseWriter.Error(w, http.StatusBadRequest, aErr)

			return
		}
		query.Activity = activity
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, lErr := strconv.Atoi(raw)
		if lErr != nil || limit < 1 {
			h.responseWriter.Error(w, http.StatusBadRequest, models.Invalid("`limit` must be a positive integer"))

			return
		}
		query.Limit = limit
	}

	picks, err := h.useCase.Do(r.Context(), query)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.ConditionPicksToHTTP(picks))
}
