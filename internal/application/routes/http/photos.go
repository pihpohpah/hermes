package http

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

// PhotoUploadHandler accepts a photo multipart upload and acknowledges it.
// Storage, moderation and URL issuance are not wired yet: the response stays
// a stub so the client flow can be built and tested end to end beforehand.
type PhotoUploadHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.PhotoUpload]
	maxBytes       int64
}

func NewPhotoUploadHandler(logger *slog.Logger, maxBytes int64) *PhotoUploadHandler {
	if maxBytes <= 0 {
		maxBytes = defaultPhotoUploadLimit
	}

	return &PhotoUploadHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.PhotoUpload](logger),
		maxBytes:       maxBytes,
	}
}

const defaultPhotoUploadLimit = 10 << 20 // 10 MB

// POST /photos — multipart/form-data with one "file" part.
func (h *PhotoUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method POST expected"))

		return
	}

	if _, err := requireUserID(r.Context()); err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes)

	file, header, err := r.FormFile("file")
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, models.Invalidf("multipart field `file`: %v", err))

		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			h.logger.Warn("photo upload: close multipart file", slog.Any("error", err))
		}
	}()

	// Consume the body now so the size limit is enforced even though the
	// bytes are not stored yet.
	discarded, err := io.Copy(io.Discard, file)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.responseWriter.Error(w, http.StatusRequestEntityTooLarge, models.Invalidf(
				"file exceeds the %d MB upload limit", h.maxBytes>>20,
			))

			return
		}

		h.responseWriter.Error(w, http.StatusBadRequest, models.Invalidf("read upload: %v", err))

		return
	}

	h.responseWriter.SuccessWithStatus(w, http.StatusAccepted, presenter.PhotoUpload{
		ID:     fmt.Sprintf("ph-%s", uploadID()),
		Status: "pending",
		Note:   fmt.Sprintf("файл `%s` (%d байт) принят, хранилище и модерация подключатся позже", header.Filename, discarded),
	})
}

// uploadID is a local random id for the stub acknowledgement; the real
// storage will issue persistent ids when it arrives.
func uploadID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}

	return hex.EncodeToString(b[:])
}
