package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

type GetPlaceForecastUC interface {
	Do(ctx context.Context, in models.ForecastQuery) ([]models.ConditionAssessment, error)
}

type GetPlaceForecastHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[[]presenter.ConditionAssessmentDTO]
	useCase        GetPlaceForecastUC
}

func NewGetPlaceForecastHandler(logger *slog.Logger, uc GetPlaceForecastUC) *GetPlaceForecastHandler {
	return &GetPlaceForecastHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[[]presenter.ConditionAssessmentDTO](logger),
		useCase:        uc,
	}
}

func (h *GetPlaceForecastHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	placeID := r.PathValue("id")
	if placeID == "" {
		h.responseWriter.Error(w, http.StatusBadRequest, models.Invalid("place id is required"))

		return
	}

	date, err := parseTargetDate(r.URL.Query().Get("date"))
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	query := models.ForecastQuery{
		PlaceID:  placeID,
		Date:     date,
		Activity: models.ActivityWalk,
		Days:     1,
	}
	if daysRaw := r.URL.Query().Get("days"); daysRaw != "" {
		days, dErr := strconv.Atoi(daysRaw)
		if dErr != nil || days < 1 || days > models.ForecastHorizonDays {
			h.responseWriter.Error(w, http.StatusBadRequest, models.Invalid(
				"days must be an integer between 1 and 10",
			))

			return
		}
		query.Days = days
	}
	if activityRaw := r.URL.Query().Get("activity"); activityRaw != "" {
		activity, aErr := models.NewActivity(activityRaw)
		if aErr != nil {
			h.responseWriter.Error(w, http.StatusBadRequest, aErr)

			return
		}
		query.Activity = activity
	}

	assessments, err := h.useCase.Do(r.Context(), query)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.ConditionAssessmentsToHTTP(assessments))
}

// parseTargetDate interprets YYYY-MM-DD in Asia/Vladivostok, not as UTC
// midnight, so the day window matches the local calendar day.
func parseTargetDate(raw string) (time.Time, error) {
	if raw == "" {
		return time.Now(), nil
	}

	window, err := models.ParseDayWindow(raw)
	if err != nil {
		return time.Time{}, err
	}

	return window.Start, nil
}

type RefreshForecastUC interface {
	Do(
		ctx context.Context,
		rng models.ForecastRange,
		activities []models.Activity,
	) (usecase.RefreshReport, error)
}

type RefreshForecastHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.ForecastRefreshResult]
	useCase        RefreshForecastUC
}

func NewRefreshForecastHandler(logger *slog.Logger, uc RefreshForecastUC) *RefreshForecastHandler {
	return &RefreshForecastHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.ForecastRefreshResult](logger),
		useCase:        uc,
	}
}

func (h *RefreshForecastHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method POST expected"))

		return
	}

	rng, err := models.NewForecastRange(time.Now())
	if err != nil {
		h.responseWriter.Error(w, http.StatusInternalServerError, err)

		return
	}

	report, err := h.useCase.Do(r.Context(), rng, nil)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.ForecastRefreshResult{
		UpdatedSlots: report.Assessments,
		RefreshedAt:  report.FinishedAt,
	})
}
