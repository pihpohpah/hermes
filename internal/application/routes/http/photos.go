package http

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

// PhotoUploadHandler accepts a photo multipart upload and registers it in the
// moderation queue. The bytes are hashed and metered now; storage, URL
// issuance and the moderation endpoint arrive with the storage layer.
type PhotoUploadHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.PhotoUpload]
	createPhoto    *usecase.CreatePhotoUseCase
	maxBytes       int64
}

func NewPhotoUploadHandler(
	logger *slog.Logger,
	createPhoto *usecase.CreatePhotoUseCase,
	maxBytes int64,
) *PhotoUploadHandler {
	if maxBytes <= 0 {
		maxBytes = defaultPhotoUploadLimit
	}

	return &PhotoUploadHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.PhotoUpload](logger),
		createPhoto:    createPhoto,
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

	userID, err := requireUserID(r.Context())
	if err != nil {
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

	hash := sha256.New()
	size, err := io.Copy(hash, file)
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

	ownerID := userID
	photo, err := h.createPhoto.Do(r.Context(), models.CreatePhotoInput{
		OwnerID:      &ownerID,
		OriginalName: header.Filename,
		ContentType:  header.Header.Get("Content-Type"),
		SizeBytes:    size,
		SHA256:       hex.EncodeToString(hash.Sum(nil)),
	})
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.SuccessWithStatus(w, http.StatusAccepted, presenter.PhotoUpload{
		ID:     photo.ID,
		Status: string(photo.Status),
		Note: fmt.Sprintf(
			"файл `%s` (%d байт) принят на модерацию, хранилище подключится позже",
			photo.OriginalName, photo.SizeBytes,
		),
	})
}
