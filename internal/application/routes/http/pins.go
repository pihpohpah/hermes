package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

// viewerID returns the authenticated user id when the request carries a
// session (Load middleware) and nil for anonymous requests.
func viewerID(ctx context.Context) *string {
	auth, ok := AuthFromContext(ctx)
	if !ok {
		return nil
	}

	return &auth.User.ID
}

// requireUserID returns the session user id or a 401-shaped error.
func requireUserID(ctx context.Context) (string, error) {
	auth, ok := AuthFromContext(ctx)
	if !ok {
		return "", models.ErrUnauthorized
	}

	return auth.User.ID, nil
}

type ListPinsUC interface {
	Do(ctx context.Context, filter models.PinFilter) ([]models.Pin, error)
}

type ListPinsHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[[]presenter.Pin]
	useCase        ListPinsUC
}

func NewListPinsHandler(logger *slog.Logger, uc ListPinsUC) *ListPinsHandler {
	return &ListPinsHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[[]presenter.Pin](logger),
		useCase:        uc,
	}
}

// GET /pins?bbox=minLon,minLat,maxLon,maxLat&kind=sport
//
// Only active pins (ends_at > now, not hidden) are returned. bbox is optional:
// without it the whole city radar is returned, limited by the use case.
func (h *ListPinsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	filter, err := parsePinFilter(r)
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}
	filter.Viewer = viewerID(r.Context())

	pins, err := h.useCase.Do(r.Context(), filter)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.PinsToHTTP(pins))
}

func parsePinFilter(r *http.Request) (models.PinFilter, error) {
	filter := models.PinFilter{}
	query := r.URL.Query()

	if raw := query.Get("bbox"); raw != "" {
		parts := strings.Split(raw, ",")
		const bboxParts = 4
		if len(parts) != bboxParts {
			return filter, models.Invalid("`bbox` must be minLon,minLat,maxLon,maxLat")
		}

		values := make([]float64, bboxParts)
		for i, part := range parts {
			parsed, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
			if err != nil {
				return filter, models.Invalid("`bbox` coordinates must be numbers")
			}
			values[i] = parsed
		}

		bounds, err := models.NewPinBounds(values[0], values[1], values[2], values[3])
		if err != nil {
			return filter, err
		}
		filter.Bounds = &bounds
	}

	if raw := query.Get("kind"); raw != "" {
		kind, err := models.NewPinKind(raw)
		if err != nil {
			return filter, err
		}
		filter.Kind = &kind
	}

	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			return filter, models.Invalid("`limit` must be a positive integer")
		}
		filter.Limit = limit
	}

	return filter, nil
}

type GetPinUC interface {
	Do(ctx context.Context, id string, viewer *string) (models.Pin, error)
}

type GetPinHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.Pin]
	useCase        GetPinUC
}

func NewGetPinHandler(logger *slog.Logger, uc GetPinUC) *GetPinHandler {
	return &GetPinHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.Pin](logger),
		useCase:        uc,
	}
}

func (h *GetPinHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	pin, err := h.useCase.Do(r.Context(), r.PathValue("id"), viewerID(r.Context()))
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.PinToHTTP(pin))
}

type CreatePinUC interface {
	Do(ctx context.Context, in models.CreatePinInput) (models.Pin, error)
}

type CreatePinHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.Pin]
	useCase        CreatePinUC
}

func NewCreatePinHandler(logger *slog.Logger, uc CreatePinUC) *CreatePinHandler {
	return &CreatePinHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.Pin](logger),
		useCase:        uc,
	}
}

type createPinRequest struct {
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	Note       string    `json:"note"`
	PlaceLabel string    `json:"place_label"`
	Lat        float64   `json:"lat"`
	Lon        float64   `json:"lon"`
	StartsAt   time.Time `json:"starts_at"`
	EndsAt     time.Time `json:"ends_at"`
	Need       *int      `json:"need"`
}

func (h *CreatePinHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method POST expected"))

		return
	}

	authorID, err := requireUserID(r.Context())
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	var body createPinRequest
	if err := decodeJSON(r, &body); err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	input, err := body.toDomain(authorID)
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	pin, err := h.useCase.Do(r.Context(), input)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.SuccessWithStatus(w, http.StatusCreated, presenter.PinToHTTP(pin))
}

func (b createPinRequest) toDomain(authorID string) (models.CreatePinInput, error) {
	kind, err := models.NewPinKind(b.Kind)
	if err != nil {
		return models.CreatePinInput{}, err
	}

	location, err := models.NewPoint(b.Lat, b.Lon)
	if err != nil {
		return models.CreatePinInput{}, err
	}

	return models.CreatePinInput{
		Kind:       kind,
		Title:      b.Title,
		Note:       b.Note,
		PlaceLabel: b.PlaceLabel,
		Location:   location,
		StartsAt:   b.StartsAt,
		EndsAt:     b.EndsAt,
		Need:       b.Need,
		AuthorID:   authorID,
	}, nil
}

type DeletePinUC interface {
	Do(ctx context.Context, id, authorID string) error
}

type DeletePinHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[any]
	useCase        DeletePinUC
}

func NewDeletePinHandler(logger *slog.Logger, uc DeletePinUC) *DeletePinHandler {
	return &DeletePinHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[any](logger),
		useCase:        uc,
	}
}

func (h *DeletePinHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method DELETE expected"))

		return
	}

	authorID, err := requireUserID(r.Context())
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	if err := h.useCase.Do(r.Context(), r.PathValue("id"), authorID); err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, nil)
}

type SetPinGoingUC interface {
	Do(ctx context.Context, in models.SetGoingInput) (models.Pin, error)
}

type SetPinGoingHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.Pin]
	useCase        SetPinGoingUC
}

func NewSetPinGoingHandler(logger *slog.Logger, uc SetPinGoingUC) *SetPinGoingHandler {
	return &SetPinGoingHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.Pin](logger),
		useCase:        uc,
	}
}

// PUT /pins/{id}/going joins the pin, DELETE leaves it. The handler serves
// both methods and passes the intent to the use case.
func (h *SetPinGoingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	going := true
	switch r.Method {
	case http.MethodPut:
	case http.MethodDelete:
		going = false
	default:
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method PUT or DELETE expected"))

		return
	}

	userID, err := requireUserID(r.Context())
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	pin, err := h.useCase.Do(r.Context(), models.SetGoingInput{
		PinID:  r.PathValue("id"),
		UserID: userID,
		Going:  going,
	})
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.PinToHTTP(pin))
}

type ReportPinUC interface {
	Do(ctx context.Context, in models.CreatePinReportInput) (models.PinReport, error)
}

type ReportPinHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.PinReport]
	useCase        ReportPinUC
}

func NewReportPinHandler(logger *slog.Logger, uc ReportPinUC) *ReportPinHandler {
	return &ReportPinHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.PinReport](logger),
		useCase:        uc,
	}
}

type reportPinRequest struct {
	Reason string `json:"reason"`
}

func (h *ReportPinHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method POST expected"))

		return
	}

	userID, err := requireUserID(r.Context())
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	var body reportPinRequest
	if err := decodeJSON(r, &body); err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	report, err := h.useCase.Do(r.Context(), models.CreatePinReportInput{
		PinID:  r.PathValue("id"),
		UserID: userID,
		Reason: body.Reason,
	})
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.SuccessWithStatus(w, http.StatusCreated, presenter.PinReportToHTTP(report))
}
