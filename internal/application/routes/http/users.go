package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

// IdempotencyHeader is the client key that makes POST /users safe to retry.
const IdempotencyHeader = "Idempotency-Key"

const maxIdempotencyKeyLength = 255

type CreateUserUC interface {
	Do(ctx context.Context, in usecase.CreateUserInput) (usecase.CreateUserResult, error)
}

type CreateUserHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.CreatedUser]
	useCase        CreateUserUC
	secureCookies  bool
}

func NewCreateUserHandler(logger *slog.Logger, uc CreateUserUC, secureCookies bool) *CreateUserHandler {
	return &CreateUserHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.CreatedUser](logger),
		useCase:        uc,
		secureCookies:  secureCookies,
	}
}

type createUserRequest struct {
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
}

func (h *CreateUserHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method POST expected"))

		return
	}

	rawBody, err := readBody(r)
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	var body createUserRequest
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, models.Invalidf("invalid JSON body: %v", err))

		return
	}

	idempotencyKey, err := idempotencyKeyFrom(r)
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	result, err := h.useCase.Do(r.Context(), usecase.CreateUserInput{
		DisplayName:        body.DisplayName,
		Timezone:           body.Timezone,
		IdempotencyKey:     idempotencyKey,
		RequestFingerprint: requestFingerprint(rawBody),
	})
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	SetSessionCookie(w, result.SessionToken, result.SessionExpiresAt, h.secureCookies)

	h.responseWriter.SuccessWithStatus(w, http.StatusCreated, presenter.CreatedUserToHTTP(result))
}

func readBody(r *http.Request) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBodyBytes+1))
	if err != nil {
		return nil, models.Invalidf("read request body: %v", err)
	}

	if len(raw) > maxJSONBodyBytes {
		return nil, models.Invalid("request body is too large")
	}

	return raw, nil
}

func idempotencyKeyFrom(r *http.Request) (string, error) {
	key := r.Header.Get(IdempotencyHeader)
	if key == "" {
		return "", nil
	}

	if len(key) > maxIdempotencyKeyLength {
		return "", models.Invalidf("%s is too long (max %d characters)", IdempotencyHeader, maxIdempotencyKeyLength)
	}

	return key, nil
}

// requestFingerprint hashes the raw body so an idempotency key can be bound
// to exactly the request it was first used with.
func requestFingerprint(raw []byte) string {
	sum := sha256.Sum256(raw)

	return hex.EncodeToString(sum[:])
}

type GetCurrentUserHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.User]
}

func NewGetCurrentUserHandler(logger *slog.Logger) *GetCurrentUserHandler {
	return &GetCurrentUserHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.User](logger),
	}
}

func (h *GetCurrentUserHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	auth, ok := AuthFromContext(r.Context())
	if !ok {
		h.responseWriter.Error(w, http.StatusUnauthorized, models.ErrUnauthorized)

		return
	}

	h.responseWriter.Success(w, presenter.UserToHTTP(auth.User))
}
