package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/web/templates"
)

// HealthCheckHandler reports whether the service can reach PostgreSQL.
type HealthCheckHandler struct {
	logger *slog.Logger
	pinger dbPinger
	writer *APIResponseWriter[healthResponse]
}

type dbPinger interface {
	Ping(ctx context.Context) error
}

type healthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

func NewHealthCheckHandler(logger *slog.Logger, pinger dbPinger) *HealthCheckHandler {
	return &HealthCheckHandler{
		logger: logger,
		pinger: pinger,
		writer: NewAPIResponseWriter[healthResponse](logger),
	}
}

func (h *HealthCheckHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.writer.Error(w, http.StatusMethodNotAllowed, errors.New("method GET or HEAD expected"))

		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
	defer cancel()

	if err := h.pinger.Ping(ctx); err != nil {
		h.logger.Error("health check: database unreachable", slog.Any("error", err))
		h.writer.Error(w, http.StatusServiceUnavailable, errors.New("database unreachable"))

		return
	}

	h.writer.Success(w, healthResponse{Status: "ok", Database: "ok"})
}

// IndexPageHandler renders the htmx shell.
type IndexPageHandler struct {
	logger *slog.Logger
}

func NewIndexPageHandler(logger *slog.Logger) *IndexPageHandler {
	return &IndexPageHandler{logger: logger}
}

func (h *IndexPageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.IndexShell().Render(r.Context(), w); err != nil {
		h.logger.Error("render index shell failed", slog.Any("error", err))
	}
}

// PlacesListFragmentHandler renders the sidebar list for htmx swaps.
type PlacesListFragmentHandler struct {
	logger  *slog.Logger
	useCase ListPlacesUC
}

func NewPlacesListFragmentHandler(logger *slog.Logger, uc ListPlacesUC) *PlacesListFragmentHandler {
	return &PlacesListFragmentHandler{
		logger:  logger,
		useCase: uc,
	}
}

func (h *PlacesListFragmentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	filter, err := parsePlaceFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	places, err := h.useCase.Do(r.Context(), filter)
	if err != nil {
		h.logger.Error("load places for fragment failed", slog.Any("error", err))
		http.Error(w, "internal error", statusCode(err))

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if renderErr := templates.PlacesList(presenter.PlacesToHTTP(places)).Render(r.Context(), w); renderErr != nil {
		h.logger.Error("render places list fragment failed", slog.Any("error", renderErr))
	}
}

// PlaceCardFragmentHandler renders a single place card.
type PlaceCardFragmentHandler struct {
	logger  *slog.Logger
	useCase GetPlaceUC
}

func NewPlaceCardFragmentHandler(logger *slog.Logger, uc GetPlaceUC) *PlaceCardFragmentHandler {
	return &PlaceCardFragmentHandler{
		logger:  logger,
		useCase: uc,
	}
}

func (h *PlaceCardFragmentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	place, err := h.useCase.Do(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), statusCode(err))

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if renderErr := templates.PlaceCard(presenter.PlaceToHTTP(place)).Render(r.Context(), w); renderErr != nil {
		h.logger.Error("render place card fragment failed", slog.Any("error", renderErr))
	}
}
