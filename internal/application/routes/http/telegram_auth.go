package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/application/usecase"
)

// TelegramLoginHandler принимает POST /auth/telegram: id_token коллбека
// Telegram Login library. Успех — session cookie в ответе и профиль с csrf_token
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
	IDToken  string `json:"id_token"`
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
		IDToken:  body.IDToken,
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

// TelegramWidgetConfigHandler отдаёт настройки для фронтенд-инициализации
// Telegram Login library. Без Client ID Telegram-вход выключен — клиент
// рисует только вход по имени. Каждый ответ несёт свежий nonce: id_token без
// выданного нами nonce сервер не примет.
type TelegramWidgetConfigHandler struct {
	clientID string
	nonces   *usecase.TelegramNonceIssuer
	writer   *APIResponseWriter[telegramWidgetConfig]
	logger   *slog.Logger
}

type telegramWidgetConfig struct {
	Enabled  bool   `json:"enabled"`
	ClientID string `json:"client_id"`
	Nonce    string `json:"nonce"`
}

func NewTelegramWidgetConfigHandler(
	logger *slog.Logger,
	clientID string,
	nonces *usecase.TelegramNonceIssuer,
) *TelegramWidgetConfigHandler {
	return &TelegramWidgetConfigHandler{
		clientID: clientID,
		nonces:   nonces,
		writer:   NewAPIResponseWriter[telegramWidgetConfig](logger),
		logger:   logger,
	}
}

// GET /auth/telegram/widget.
func (h *TelegramWidgetConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writer.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	if h.clientID == "" {
		h.writer.Success(w, telegramWidgetConfig{Enabled: false})

		return
	}

	nonce, err := h.nonces.Issue()
	if err != nil {
		h.logger.Error("issue telegram nonce failed", slog.Any("error", err))
		h.writer.Error(w, http.StatusInternalServerError, errors.New("nonce issue failed"))

		return
	}

	h.writer.Success(w, telegramWidgetConfig{
		Enabled:  true,
		ClientID: h.clientID,
		Nonce:    nonce,
	})
}
