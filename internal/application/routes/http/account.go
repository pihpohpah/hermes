package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

// sessionRevoker закрывает сессию текущего пользователя.
type sessionRevoker interface {
	Revoke(ctx context.Context, id string, now time.Time) error
}

// LogoutHandler завершает сессию и сбрасывает cookie.
type LogoutHandler struct {
	logger  *slog.Logger
	revoke  sessionRevoker
	writer  *APIResponseWriter[any]
	secure  bool
}

func NewLogoutHandler(logger *slog.Logger, revoke sessionRevoker, secure bool) *LogoutHandler {
	return &LogoutHandler{
		logger: logger,
		revoke: revoke,
		writer: NewAPIResponseWriter[any](logger),
		secure: secure,
	}
}

// POST /users/logout.
func (h *LogoutHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writer.Error(w, http.StatusMethodNotAllowed, errors.New("method POST expected"))

		return
	}

	auth, ok := AuthFromContext(r.Context())
	if !ok {
		h.writer.Error(w, http.StatusUnauthorized, models.ErrUnauthorized)

		return
	}

	if err := h.revoke.Revoke(r.Context(), auth.Session.ID, time.Now()); err != nil {
		h.logger.Error("session revoke failed", slog.Any("error", err))
	}

	clearSessionCookie(w, h.secure)
	h.writer.Success(w, nil)
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

// displayNameUpdater меняет имя профиля.
type displayNameUpdater interface {
	UpdateDisplayName(ctx context.Context, id, displayName string) (models.User, error)
}

// UpdateCurrentUserHandler — PATCH /users/me: смена отображаемого имени.
type UpdateCurrentUserHandler struct {
	logger *slog.Logger
	update displayNameUpdater
	writer *APIResponseWriter[presenter.User]
}

func NewUpdateCurrentUserHandler(logger *slog.Logger, update displayNameUpdater) *UpdateCurrentUserHandler {
	return &UpdateCurrentUserHandler{
		logger: logger,
		update: update,
		writer: NewAPIResponseWriter[presenter.User](logger),
	}
}

func (h *UpdateCurrentUserHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		h.writer.Error(w, http.StatusMethodNotAllowed, errors.New("method PATCH expected"))

		return
	}

	auth, ok := AuthFromContext(r.Context())
	if !ok {
		h.writer.Error(w, http.StatusUnauthorized, models.ErrUnauthorized)

		return
	}

	var body struct {
		DisplayName string `json:"display_name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		h.writer.Error(w, http.StatusBadRequest, err)

		return
	}

	name := strings.TrimSpace(body.DisplayName)
	if len([]rune(name)) < 2 {
		h.writer.Error(w, http.StatusBadRequest, models.Invalid("display_name must be at least 2 characters"))

		return
	}

	user, err := h.update.UpdateDisplayName(r.Context(), auth.User.ID, name)
	if err != nil {
		h.writer.Error(w, statusCode(err), err)

		return
	}

	h.writer.Success(w, presenter.UserToHTTP(user))
}
