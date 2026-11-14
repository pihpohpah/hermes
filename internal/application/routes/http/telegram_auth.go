package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

// TelegramLoginHandler принимает POST /auth/telegram: подписанный Telegram
// Login Widget коллбек. Успех — session cookie в ответе и профиль с csrf_token
// в теле, ровно как у POST /users.
type TelegramLoginHandler struct {
	logger        *slog.Logger
	useCase       *usecase.TelegramLoginUseCase
	writer        *APIResponseWriter[presenter.CreatedUser]
	secureCookies bool
}

func NewTelegramLoginHandler(
	logger *slog.Logger,
	uc *usecase.TelegramLoginUseCase,
	secureCookies bool,
) *TelegramLoginHandler {
	return &TelegramLoginHandler{
		logger:        logger,
		useCase:       uc,
		writer:        NewAPIResponseWriter[presenter.CreatedUser](logger),
		secureCookies: secureCookies,
	}
}

type telegramLoginRequest struct {
	models.TelegramAuthFields
	Timezone string `json:"timezone"`
}

// POST /auth/telegram.
func (h *TelegramLoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writer.Error(w, http.StatusMethodNotAllowed, errors.New("method POST expected"))

		return
	}

	var body telegramLoginRequest
	if err := decodeJSON(r, &body); err != nil {
		h.writer.Error(w, http.StatusBadRequest, err)

		return
	}

	result, err := h.useCase.Do(r.Context(), usecase.TelegramLoginInput{
		Telegram: body.TelegramAuthFields,
		Timezone: body.Timezone,
	})
	if err != nil {
		h.writer.Error(w, statusCode(err), err)

		return
	}

	SetSessionCookie(w, result.SessionToken, result.SessionExpiresAt, h.secureCookies)

	h.writer.SuccessWithStatus(w, http.StatusCreated, presenter.CreatedUserToHTTP(usecase.CreateUserResult{
		User:             result.User,
		SessionToken:     result.SessionToken,
		CSRFToken:        result.CSRFToken,
		SessionExpiresAt: result.SessionExpiresAt,
	}))
}

// TelegramWidgetConfigHandler отдаёт настройки виджета для фронтенда. Без
// токена бота Telegram-вход выключен — клиент рисует только вход по имени.
type TelegramWidgetConfigHandler struct {
	botUsername string
	writer      *APIResponseWriter[telegramWidgetConfig]
}

type telegramWidgetConfig struct {
	Enabled    bool   `json:"enabled"`
	BotUsername string `json:"bot_username"`
}

func NewTelegramWidgetConfigHandler(logger *slog.Logger, botUsername string) *TelegramWidgetConfigHandler {
	return &TelegramWidgetConfigHandler{
		botUsername: botUsername,
		writer:      NewAPIResponseWriter[telegramWidgetConfig](logger),
	}
}

// GET /auth/telegram/widget.
func (h *TelegramWidgetConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writer.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	h.writer.Success(w, telegramWidgetConfig{
		Enabled:    h.botUsername != "",
		BotUsername: h.botUsername,
	})
}
