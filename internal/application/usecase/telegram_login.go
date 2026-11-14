package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// TelegramLoginInput — всё, что нужно для входа через Telegram Login library.
// IDToken — OIDC id_token из её коллбека; use case проверяет подпись по JWKS
// и свежесть, Timezone — обычное поле профиля из браузера клиента.
type TelegramLoginInput struct {
	IDToken  string
	Timezone string
}

// TelegramLoginResult повторяет контракт создания пользователя: профиль плюс
// свежая сессия. Created отличает первый вход (создали профиль и привязку)
// от повторного (только сессия).
type TelegramLoginResult struct {
	User             models.User
	SessionToken     string
	CSRFToken        string
	SessionExpiresAt time.Time
	Created          bool
}

type identityStore interface {
	GetByProviderSubject(ctx context.Context, provider, subject string) (models.UserIdentity, error)
	Create(ctx context.Context, identity models.UserIdentity) (models.UserIdentity, error)
}

// idTokenVerifier — инфраструктурная проверка id_token Telegram: подпись RS256
// по JWKS, iss, aud, exp. Реализует *telegram.IDTokenVerifier.
type idTokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (models.TelegramIDClaims, error)
}

// TelegramLoginUseCase принимает id_token коллбека Telegram Login library,
// находит или создаёт профиль по identity (tgId — источник входа; телефон
// пользователь может добавить в аккаунте позже) и выдаёт сессию.
type TelegramLoginUseCase struct {
	clientID string
	verifier idTokenVerifier
	nonces   *TelegramNonceIssuer
	users    userCreator
	ids      identityStore
	sessions sessionCreator
	tx       txRunner
	now      func() time.Time
	ttl      time.Duration
}

func NewTelegramLoginUseCase(
	clientID string,
	verifier idTokenVerifier,
	nonces *TelegramNonceIssuer,
	users userCreator,
	ids identityStore,
	sessions sessionCreator,
	tx txRunner,
	ttl time.Duration,
) *TelegramLoginUseCase {
	return &TelegramLoginUseCase{
		clientID: clientID,
		verifier: verifier,
		nonces:   nonces,
		users:    users,
		ids:      ids,
		sessions: sessions,
		tx:       tx,
		now:      time.Now,
		ttl:      ttl,
	}
}

func (u *TelegramLoginUseCase) Do(ctx context.Context, in TelegramLoginInput) (TelegramLoginResult, error) {
	if u.clientID == "" {
		return TelegramLoginResult{}, errors.New("telegram login is not configured")
	}

	claims, err := u.verify(ctx, in.IDToken)
	if err != nil {
		return TelegramLoginResult{}, err
	}

	user, created, err := u.resolveUser(ctx, claims, in.Timezone)
	if err != nil {
		return TelegramLoginResult{}, err
	}

	result := TelegramLoginResult{User: user, Created: created}

	err = u.tx.Run(ctx, func(txCtx context.Context) error {
		sessionToken, csrfToken, expiresAt, err := issueSession(txCtx, u.sessions, user.ID, u.now(), u.ttl)
		if err != nil {
			return err
		}

		result.SessionToken = sessionToken
		result.CSRFToken = csrfToken
		result.SessionExpiresAt = expiresAt

		return nil
	})
	if err != nil {
		return TelegramLoginResult{}, err
	}

	return result, nil
}

// verify перепроверяет id_token полностью: подпись, iss, aud и exp — в
// верификаторе; nonce — выданный нами и ещё не просроченный. Без доверенного
// токена claims дальше не идут никогда.
func (u *TelegramLoginUseCase) verify(ctx context.Context, rawToken string) (models.TelegramIDClaims, error) {
	if rawToken == "" {
		return models.TelegramIDClaims{}, models.Invalid("`id_token` is required")
	}

	claims, err := u.verifier.Verify(ctx, rawToken)
	if err != nil {
		return models.TelegramIDClaims{}, models.Invalidf("invalid telegram id_token: %v", err)
	}

	if claims.IdentitySubject() == "" {
		return models.TelegramIDClaims{}, models.Invalid("telegram id_token has no user id")
	}

	if claims.Nonce == "" || !u.nonces.Valid(claims.Nonce) {
		return models.TelegramIDClaims{}, models.Invalid("telegram id_token has no valid nonce")
	}

	return claims, nil
}

// resolveUser находит профиль по (telegram, tgId). Первый вход создаёт и
// профиль, и привязку в одной транзакции; гонка двух первых входов с одним
// tgId проигрывает вставку уникальности и перечитывает чужой результат.
func (u *TelegramLoginUseCase) resolveUser(
	ctx context.Context,
	claims models.TelegramIDClaims,
	timezone string,
) (models.User, bool, error) {
	subject := claims.IdentitySubject()

	identity, err := u.ids.GetByProviderSubject(ctx, models.IdentityProviderTelegram, subject)
	if err == nil {
		user, err := u.users.GetByID(ctx, identity.UserID)
		if err != nil {
			return models.User{}, false, fmt.Errorf("load user by telegram identity: %w", err)
		}

		return user, false, nil
	}

	if !errors.Is(err, models.ErrNotFound) {
		return models.User{}, false, fmt.Errorf("load telegram identity: %w", err)
	}

	user, err := u.createUser(ctx, claims, timezone, subject)
	if err == nil {
		return user, true, nil
	}

	// Параллельный первый вход с тем же tgId успел раньше: перечитываем.
	if errors.Is(err, models.ErrConflict) {
		identity, lookupErr := u.ids.GetByProviderSubject(ctx, models.IdentityProviderTelegram, subject)
		if lookupErr == nil {
			user, userErr := u.users.GetByID(ctx, identity.UserID)
			if userErr == nil {
				return user, false, nil
			}
		}
	}

	return models.User{}, false, err
}

// createUser атомарно создаёт профиль с именем из Telegram и привязку к tgId.
func (u *TelegramLoginUseCase) createUser(
	ctx context.Context,
	claims models.TelegramIDClaims,
	timezone string,
	subject string,
) (models.User, error) {
	var created models.User

	err := u.tx.Run(ctx, func(txCtx context.Context) error {
		user, err := models.NewUser(
			models.TelegramDisplayName(claims.IdentityData()),
			timezone,
		)
		if err != nil {
			return err
		}

		user, err = u.users.Create(txCtx, user)
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}

		identity, err := models.NewUserIdentity(
			user.ID,
			models.IdentityProviderTelegram,
			subject,
			claims.IdentityData(),
		)
		if err != nil {
			return err
		}

		if _, err := u.ids.Create(txCtx, identity); err != nil {
			return fmt.Errorf("create telegram identity: %w", err)
		}

		created = user

		return nil
	})
	if err != nil {
		return models.User{}, err
	}

	return created, nil
}

// issueSession persists a session for userID and returns the opaque tokens
// together with the expiry. It is shared by the user-creation use case and
// the telegram login and must be called inside a txRunner.Run.
func issueSession(
	ctx context.Context,
	sessions sessionCreator,
	userID string,
	now time.Time,
	ttl time.Duration,
) (sessionToken string, csrfToken string, expiresAt time.Time, err error) {
	sessionToken, sessionHash, err := models.NewOpaqueToken()
	if err != nil {
		return "", "", time.Time{}, err
	}

	csrfToken, csrfHash, err := models.NewOpaqueToken()
	if err != nil {
		return "", "", time.Time{}, err
	}

	session, err := models.NewUserSession(userID, sessionHash, csrfHash, now, ttl)
	if err != nil {
		return "", "", time.Time{}, err
	}

	stored, err := sessions.Create(ctx, session)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("create session: %w", err)
	}

	return sessionToken, csrfToken, stored.ExpiresAt, nil
}
