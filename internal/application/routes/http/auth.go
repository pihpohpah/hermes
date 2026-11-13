package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// SessionCookieName is the cookie that carries the opaque session token.
const SessionCookieName = "hermes_session"

// csrfHeader carries the raw CSRF token on mutating requests.
const csrfHeader = "X-CSRF-Token"

type authContextKey struct{}

// AuthContext is what the session middleware proves and hands to handlers:
// the authenticated user and the session it came from.
type AuthContext struct {
	User    models.User
	Session models.UserSession
}

type sessionLoader interface {
	FindActiveByTokenHash(ctx context.Context, tokenHash string, now time.Time) (models.UserSession, error)
}

type sessionToucher interface {
	Touch(ctx context.Context, id string, now time.Time) error
}

type userGetter interface {
	GetByID(ctx context.Context, id string) (models.User, error)
}

// AuthMiddleware resolves the session cookie into an AuthContext. Require
// rejects requests without a valid session with 401 and enforces the CSRF
// header on mutating methods; Load only populates the context when a valid
// session exists and is used for endpoints that work anonymously today.
type AuthMiddleware struct {
	logger   *slog.Logger
	sessions sessionLoader
	touch    sessionToucher
	users    userGetter
	now      func() time.Time
}

func NewAuthMiddleware(
	logger *slog.Logger,
	sessions sessionLoader,
	touch sessionToucher,
	users userGetter,
) *AuthMiddleware {
	return &AuthMiddleware{
		logger:   logger,
		sessions: sessions,
		touch:    touch,
		users:    users,
		now:      time.Now,
	}
}

// Load attaches the AuthContext when the request carries a valid session
// cookie; anonymous requests pass through untouched.
func (m *AuthMiddleware) Load(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, m.maybeAttach(r))
	})
}

// Require rejects anonymous requests with 401. For mutating methods it also
// requires the X-CSRF-Token header to match the session (403 otherwise): the
// cookie is SameSite=Lax, but an extra explicit check keeps cookie-authenticated
// writes safe cross-site even where Lax would still send the cookie.
func (m *AuthMiddleware) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, ok := m.resolve(r)
		if !ok {
			writeAuthError(w, http.StatusUnauthorized, models.ErrUnauthorized)

			return
		}

		if isMutating(r.Method) && !auth.Session.CSRFMatches(r.Header.Get(csrfHeader)) {
			writeAuthError(w, http.StatusForbidden, models.ErrForbidden)

			return
		}

		next.ServeHTTP(w, r.WithContext(ContextWithAuth(r.Context(), auth)))
	})
}

func (m *AuthMiddleware) maybeAttach(r *http.Request) *http.Request {
	auth, ok := m.resolve(r)
	if !ok {
		return r
	}

	return r.WithContext(ContextWithAuth(r.Context(), auth))
}

// resolve authenticates the session cookie. Any failure — missing cookie,
// unknown, expired or revoked token — collapses to "not authenticated"; the
// reason is logged but never leaked to the client.
func (m *AuthMiddleware) resolve(r *http.Request) (AuthContext, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return AuthContext{}, false
	}

	now := m.now()

	session, err := m.sessions.FindActiveByTokenHash(r.Context(), models.HashToken(cookie.Value), now)
	if err != nil {
		if err != models.ErrNotFound {
			m.logger.Warn("session lookup failed", slog.Any("error", err))
		}

		return AuthContext{}, false
	}

	user, err := m.users.GetByID(r.Context(), session.UserID)
	if err != nil {
		m.logger.Warn("session user lookup failed", slog.Any("error", err))

		return AuthContext{}, false
	}

	if err := m.touch.Touch(r.Context(), session.ID, now); err != nil {
		m.logger.Warn("session touch failed", slog.Any("error", err))
	}

	return AuthContext{User: user, Session: session}, true
}

func ContextWithAuth(ctx context.Context, auth AuthContext) context.Context {
	return context.WithValue(ctx, authContextKey{}, auth)
}

func AuthFromContext(ctx context.Context) (AuthContext, bool) {
	auth, ok := ctx.Value(authContextKey{}).(AuthContext)

	return auth, ok
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func writeAuthError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"success":false,"error":"` + err.Error() + `","code":"` + errorCode(status, err) + `","data":null}`))
}

// SetSessionCookie installs the session cookie with the hardening the task
// requires: HttpOnly against XSS, SameSite=Lax as the default, Secure only in
// production so plain-HTTP local dev keeps working.
func SetSessionCookie(w http.ResponseWriter, token string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}
