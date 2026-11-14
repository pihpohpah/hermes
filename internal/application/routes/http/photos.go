package http

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
	"github.com/pihpohpah/hermes/internal/infrastructure/photostore"
)

// PhotoUploadHandler accepts a photo multipart upload, stores the processed
// JPEG variants on disk and registers the row. The submission review covers
// photos as well, so uploads become addressable right away.
type PhotoUploadHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.PhotoUpload]
	createPhoto    *usecase.CreatePhotoUseCase
	store          *photostore.Store
	maxBytes       int64
}

func NewPhotoUploadHandler(
	logger *slog.Logger,
	createPhoto *usecase.CreatePhotoUseCase,
	store *photostore.Store,
	maxBytes int64,
) *PhotoUploadHandler {
	if maxBytes <= 0 {
		maxBytes = defaultPhotoUploadLimit
	}

	return &PhotoUploadHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.PhotoUpload](logger),
		createPhoto:    createPhoto,
		store:          store,
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
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.responseWriter.Error(w, http.StatusRequestEntityTooLarge, models.Invalidf(
				"file exceeds the %d MB upload limit", h.maxBytes>>20,
			))

			return
		}

		h.responseWriter.Error(w, http.StatusBadRequest, models.Invalidf("multipart field `file`: %v", err))

		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			h.logger.Warn("photo upload: close multipart file", slog.Any("error", err))
		}
	}()

	data, err := io.ReadAll(file)
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

	id, err := models.NewUUIDv7(time.Now())
	if err != nil {
		h.responseWriter.Error(w, http.StatusInternalServerError, err)

		return
	}

	saved, err := h.store.Save(r.Context(), id, data)
	if err != nil {
		h.logger.Warn("photo upload: store file", slog.String("id", id), slog.Any("error", err))
		h.responseWriter.Error(w, http.StatusBadRequest, models.Invalidf(
			"не получилось обработать файл как изображение (JPG, PNG, WebP до %d МБ)", h.maxBytes>>20,
		))

		return
	}

	sum := sha256.Sum256(data)
	ownerID := userID
	photo, err := h.createPhoto.Do(r.Context(), models.CreatePhotoInput{
		ID:           id,
		OwnerID:      &ownerID,
		Status:       models.PhotoStatusActive,
		StoragePath:  saved.Full.Path,
		OriginalName: header.Filename,
		ContentType:  "image/jpeg",
		SizeBytes:    int64(len(data)),
		Width:        &saved.Full.Width,
		Height:       &saved.Full.Height,
		SHA256:       hex.EncodeToString(sum[:]),
	})
	if err != nil {
		// Строки нет — файлы без владельца не оставляем.
		if removeErr := h.store.Remove(r.Context(), saved); removeErr != nil {
			h.logger.Warn("photo upload: remove orphaned files", slog.String("id", id), slog.Any("error", removeErr))
		}
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.SuccessWithStatus(w, http.StatusCreated, presenter.PhotoUpload{
		ID:     photo.ID,
		Status: string(photo.Status),
		URL:    saved.Full.URL,
		URLSm:  saved.Small.URL,
		Width:  saved.Full.Width,
		Height: saved.Full.Height,
		Note: fmt.Sprintf(
			"файл `%s` (%d байт) принят: %dx%d",
			photo.OriginalName, photo.SizeBytes, saved.Full.Width, saved.Full.Height,
		),
	})
}
