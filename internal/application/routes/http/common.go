package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// APIResponse is the single envelope every JSON endpoint returns, so the front
// end can branch on `success` without inspecting status codes.
type APIResponse[data any] struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Data    data   `json:"data"`
}

type APIResponseWriter[data any] struct {
	logger *slog.Logger
}

func NewAPIResponseWriter[data any](logger *slog.Logger) *APIResponseWriter[data] {
	return &APIResponseWriter[data]{
		logger: logger,
	}
}

func (a *APIResponseWriter[data]) Error(writer http.ResponseWriter, code int, err error) {
	a.logger.Error("request failed", slog.Any("error", err), slog.Int("status", code))

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(code)

	if encodeErr := json.NewEncoder(writer).Encode(APIResponse[any]{
		Success: false,
		Error:   err.Error(),
		Code:    errorCode(code, err),
		Data:    nil,
	}); encodeErr != nil {
		a.logger.Error("json encode of error response failed", slog.Any("error", encodeErr))
	}
}

func (a *APIResponseWriter[data]) Success(writer http.ResponseWriter, d data) {
	a.SuccessWithStatus(writer, http.StatusOK, d)
}

func (a *APIResponseWriter[data]) SuccessWithStatus(writer http.ResponseWriter, code int, d data) {
	r := APIResponse[data]{
		Success: true,
		Error:   "",
		Data:    d,
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(code)
	if err := json.NewEncoder(writer).Encode(r); err != nil {
		a.logger.Error("json encode of success response failed", slog.Any("error", err))
	}
}

// maxJSONBodyBytes caps request bodies so a malformed client cannot exhaust memory.
const maxJSONBodyBytes = 1 << 20

const statusClientClosed = 499

func statusCode(err error) int {
	switch {
	case errors.Is(err, models.ErrInvalidArgument):
		return http.StatusBadRequest
	case errors.Is(err, models.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, models.ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, models.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, models.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, context.Canceled):
		return statusClientClosed
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

func errorCode(httpStatus int, err error) string {
	switch {
	case errors.Is(err, models.ErrInvalidArgument) || httpStatus == http.StatusBadRequest:
		return "INVALID_ARGUMENT"
	case errors.Is(err, models.ErrNotFound) || httpStatus == http.StatusNotFound:
		return "NOT_FOUND"
	case errors.Is(err, models.ErrUnauthorized) || httpStatus == http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case errors.Is(err, models.ErrForbidden) || httpStatus == http.StatusForbidden:
		return "FORBIDDEN"
	case errors.Is(err, models.ErrConflict) || httpStatus == http.StatusConflict:
		return "CONFLICT"
	case httpStatus == http.StatusMethodNotAllowed:
		return "METHOD_NOT_ALLOWED"
	case errors.Is(err, context.DeadlineExceeded) || httpStatus == http.StatusGatewayTimeout:
		return "TIMEOUT"
	case errors.Is(err, context.Canceled) || httpStatus == statusClientClosed:
		return "CLIENT_CLOSED"
	default:
		return "INTERNAL_ERROR"
	}
}

// decodeJSON reads a JSON body into dst, rejecting unknown fields so a typo in
// the payload fails loudly instead of being silently ignored.
func decodeJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxJSONBodyBytes))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return models.Invalidf("invalid JSON body: %v", err)
	}

	return nil
}
