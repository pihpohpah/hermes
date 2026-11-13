package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

type CreateSubmissionUC interface {
	Do(ctx context.Context, in models.CreateSubmissionInput) (usecase.CreateSubmissionResult, error)
}

type CreateSubmissionHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.Submission]
	useCase        CreateSubmissionUC
}

func NewCreateSubmissionHandler(logger *slog.Logger, uc CreateSubmissionUC) *CreateSubmissionHandler {
	return &CreateSubmissionHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.Submission](logger),
		useCase:        uc,
	}
}

// POST /submissions — a new place, event or fix. Optional Idempotency-Key
// header makes retries safe: same key + same body returns the stored entry.
func (h *CreateSubmissionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method POST expected"))

		return
	}

	authorID, err := requireUserID(r.Context())
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	rawBody, err := readBody(r)
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	var body createSubmissionRequest
	if err := decodeStrict(rawBody, &body); err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	input, err := body.toDomain(authorID)
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	idempotencyKey, err := idempotencyKeyFrom(r)
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}
	input.IdempotencyKey = idempotencyKey
	input.RequestFingerprint = requestFingerprint(rawBody)

	result, err := h.useCase.Do(r.Context(), input)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}

	h.responseWriter.SuccessWithStatus(w, status, presenter.SubmissionToHTTP(result.Submission))
}

// decodeStrict unmarshals a body that was already read for fingerprinting;
// unknown fields are rejected so a payload typo fails loudly.
func decodeStrict(raw []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return models.Invalidf("invalid JSON body: %v", err)
	}

	return nil
}

type createSubmissionRequest struct {
	Type        string         `json:"type"`
	Scenario    *string        `json:"scenario"`
	Category    *string        `json:"category"`
	Title       string         `json:"title"`
	Address     string         `json:"address"`
	Coords      []float64      `json:"coords"`
	Date        string         `json:"date"`
	Time        string         `json:"time"`
	Description string         `json:"description"`
	Hours       string         `json:"hours"`
	Price       string         `json:"price"`
	Link        string         `json:"link"`
	FixID       string         `json:"fix_id"`
	FixReasons  []string       `json:"fix_reasons"`
	Contact     string         `json:"contact"`
	Photos      []models.Photo `json:"photos"`
}

func (b createSubmissionRequest) toDomain(authorID string) (models.CreateSubmissionInput, error) {
	submissionType, err := models.NewSubmissionType(b.Type)
	if err != nil {
		return models.CreateSubmissionInput{}, err
	}

	input := models.CreateSubmissionInput{
		Type:        submissionType,
		Category:    b.Category,
		Title:       b.Title,
		Address:     b.Address,
		EventTime:   b.Time,
		Description: b.Description,
		Hours:       b.Hours,
		Price:       b.Price,
		Link:        b.Link,
		FixID:       b.FixID,
		FixReasons:  b.FixReasons,
		Contact:     b.Contact,
		Photos:      b.Photos,
		AuthorID:    authorID,
	}

	if b.Scenario != nil && strings.TrimSpace(*b.Scenario) != "" {
		layer, err := models.NewLayerCode(*b.Scenario)
		if err != nil {
			return models.CreateSubmissionInput{}, err
		}
		input.Scenario = &layer
	}

	if len(b.Coords) > 0 {
		if len(b.Coords) != 2 {
			return models.CreateSubmissionInput{}, models.Invalid("`coords` must be [lng, lat]")
		}
		location, err := models.NewPoint(b.Coords[1], b.Coords[0])
		if err != nil {
			return models.CreateSubmissionInput{}, err
		}
		input.Location = &location
	}

	if strings.TrimSpace(b.Date) != "" {
		date, err := models.ParseDayWindow(b.Date)
		if err != nil {
			return models.CreateSubmissionInput{}, err
		}
		input.EventDate = &date.Start
	}

	return input, nil
}

type GetSubmissionUC interface {
	Do(ctx context.Context, id, authorID string) (models.Submission, error)
}

type GetSubmissionHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.Submission]
	useCase        GetSubmissionUC
}

func NewGetSubmissionHandler(logger *slog.Logger, uc GetSubmissionUC) *GetSubmissionHandler {
	return &GetSubmissionHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.Submission](logger),
		useCase:        uc,
	}
}

func (h *GetSubmissionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	authorID, err := requireUserID(r.Context())
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	submission, err := h.useCase.Do(r.Context(), r.PathValue("id"), authorID)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.SubmissionToHTTP(submission))
}

type ListSubmissionsUC interface {
	Do(ctx context.Context, filter models.SubmissionFilter) ([]models.Submission, error)
}

type ListSubmissionsHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[[]presenter.Submission]
	useCase        ListSubmissionsUC
}

func NewListSubmissionsHandler(logger *slog.Logger, uc ListSubmissionsUC) *ListSubmissionsHandler {
	return &ListSubmissionsHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[[]presenter.Submission](logger),
		useCase:        uc,
	}
}

// GET /submissions?status=review — the caller's own proposals.
func (h *ListSubmissionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	authorID, err := requireUserID(r.Context())
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	filter := models.SubmissionFilter{AuthorID: authorID}

	if raw := r.URL.Query().Get("status"); raw != "" {
		status, err := models.NewSubmissionStatus(raw)
		if err != nil {
			h.responseWriter.Error(w, http.StatusBadRequest, err)

			return
		}
		filter.Status = &status
	}

	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			h.responseWriter.Error(w, http.StatusBadRequest, models.Invalid("`limit` must be a positive integer"))

			return
		}
		filter.Limit = limit
	}

	submissions, err := h.useCase.Do(r.Context(), filter)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.SubmissionsToHTTP(submissions))
}
