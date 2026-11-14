package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
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
