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

type ListReportsUC interface {
	Do(ctx context.Context, filter models.ReportFilter) ([]models.Report, error)
}

type ListReportsHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[[]presenter.Report]
	useCase        ListReportsUC
}

func NewListReportsHandler(logger *slog.Logger, uc ListReportsUC) *ListReportsHandler {
	return &ListReportsHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[[]presenter.Report](logger),
		useCase:        uc,
	}
}

func (h *ListReportsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	query := r.URL.Query()

	filter := models.ReportFilter{PlaceID: query.Get("place_id")}
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			h.responseWriter.Error(w, http.StatusBadRequest, models.Invalid("`limit` must be an integer"))

			return
		}

		filter.Limit = parsed
	}

	reports, err := h.useCase.Do(r.Context(), filter)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.ReportsToHTTP(reports))
}

type CreateReportUC interface {
	Do(ctx context.Context, in models.CreateReportInput) (models.Report, error)
}

type CreateReportHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.Report]
	useCase        CreateReportUC
}

func NewCreateReportHandler(logger *slog.Logger, uc CreateReportUC) *CreateReportHandler {
	return &CreateReportHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.Report](logger),
		useCase:        uc,
	}
}

type createReportRequest struct {
	PlaceID string  `json:"place_id"`
	UserID  *string `json:"user_id"`
	Type    string  `json:"type"`
	Comment string  `json:"comment"`
}

func (h *CreateReportHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method POST expected"))

		return
	}

	var body createReportRequest
	if err := decodeJSON(r, &body); err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	reportType, err := models.NewReportType(body.Type)
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	report, err := h.useCase.Do(r.Context(), models.CreateReportInput{
		PlaceID: body.PlaceID,
		UserID:  body.UserID,
		Type:    reportType,
		Comment: body.Comment,
	})
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.SuccessWithStatus(w, http.StatusCreated, presenter.ReportToHTTP(report))
}
