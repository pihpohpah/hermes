package http

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"
)

const healthCheckTimeout = 3 * time.Second

// PMTilesHandler отдаёт офлайн-архив карты.
//
// PMTiles-клиент читает архив произвольными диапазонами (заголовок, директории,
// отдельные тайлы), поэтому байтовые диапазоны здесь обязательны: без них
// MapLibre вынужден качать весь архив целиком.
type PMTilesHandler struct {
	logger         *slog.Logger
	filePath       string
	responseWriter *APIResponseWriter[any]
}

func NewPMTilesHandler(logger *slog.Logger, filePath string) *PMTilesHandler {
	return &PMTilesHandler{
		logger:         logger,
		filePath:       filePath,
		responseWriter: NewAPIResponseWriter[any](logger),
	}
}

func (h *PMTilesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET or HEAD expected"))

		return
	}

	if _, err := os.Stat(h.filePath); err != nil {
		h.logger.Error("pmtiles archive unavailable", slog.String("path", h.filePath), slog.Any("error", err))
		h.responseWriter.Error(w, http.StatusServiceUnavailable, errors.New("map archive is not built yet"))

		return
	}

	w.Header().Set("Content-Type", "application/vnd.pmtiles")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	http.ServeFile(w, r, h.filePath)
}
