package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// telegramAuthMaxAge отсекает подписанные виджетом данные, которым больше
// суток: перехваченный коллбек не должен давать вход вечно.
const telegramAuthMaxAge = 24 * time.Hour

// TelegramLoginInput — всё, что нужно для входа через Telegram Login Widget.
// Поля Telegram проверяются по подписи; Timezone — обычное поле профиля из
// браузера клиента.
type TelegramLoginInput struct {
	Telegram models.TelegramAuthFields
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

// TelegramLoginUseCase принимает подписанный виджетом коллбек Telegram,
// находит или создаёт профиль по identity (tgId — источник входа; телефон
// пользователь может добавить в аккаунте позже) и выдаёт сессию.
type TelegramLoginUseCase struct {
	botToken string
	users    userCreator
	ids      identityStore
	sessions sessionCreator
	tx       txRunner
	now      func() time.Time
	ttl      time.Duration
}

func NewTelegramLoginUseCase(
	botToken string,
	users userCreator,
	ids identityStore,
	sessions sessionCreator,
	tx txRunner,
	ttl time.Duration,
) *TelegramLoginUseCase {
	return &TelegramLoginUseCase{
		botToken: botToken,
		users:    users,
		ids:      ids,
		sessions: sessions,
		tx:       tx,
		now:      time.Now,
		ttl:      ttl,
	}
}

func (u *TelegramLoginUseCase) Do(ctx context.Context, in TelegramLoginInput) (TelegramLoginResult, error) {
	if u.botToken == "" {
		return TelegramLoginResult{}, errors.New("telegram login is not configured")
	}

	if err := u.verify(in.Telegram); err != nil {
		return TelegramLoginResult{}, err
	}

	user, created, err := u.resolveUser(ctx, in)
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

// verify повторяет проверку подписи из документации Login Widget:
// secret_key = SHA256(bot_token), hash = HMAC_SHA256(secret_key,
// data_check_string). Подпись и свежесть auth_date проверяются в constant time.
func (u *TelegramLoginUseCase) verify(fields models.TelegramAuthFields) error {
	if fields.ID <= 0 {
		return models.Invalid("`id` must be a positive telegram user id")
	}

	if fields.Hash == "" {
		return models.Invalid("`hash` is required")
	}

	if fields.AuthDate <= 0 {
		return models.Invalid("`auth_date` is required")
	}

	if age := u.now().Sub(time.Unix(fields.AuthDate, 0)); age > telegramAuthMaxAge || age < -time.Minute {
		return models.Invalidf("telegram auth data is stale (age %s)", age.Truncate(time.Second))
	}

	sum := sha256.Sum256([]byte(u.botToken))
	mac := hmac.New(sha256.New, sum[:])
	_, _ = mac.Write([]byte(fields.DataCheckString()))
	given, err := hex.DecodeString(fields.Hash)
	if err != nil || !hmac.Equal(given, mac.Sum(nil)) {
		return models.Invalid("invalid telegram signature")
	}

	return nil
}

// resolveUser находит профиль по (telegram, tgId). Первый вход создаёт и
// профиль, и привязку в одной транзакции; гонка двух первых входов с одним
// tgId проигрывает вставку уникальности и перечитывает чужой результат.
func (u *TelegramLoginUseCase) resolveUser(
	ctx context.Context,
	in TelegramLoginInput,
) (models.User, bool, error) {
	subject := strconv.FormatInt(in.Telegram.ID, 10)

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

	user, err := u.createUser(ctx, in, subject)
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
	in TelegramLoginInput,
	subject string,
) (models.User, error) {
	var created models.User

	err := u.tx.Run(ctx, func(txCtx context.Context) error {
		user, err := models.NewUser(
			models.TelegramDisplayName(in.Telegram.IdentityData()),
			in.Timezone,
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
			in.Telegram.IdentityData(),
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
