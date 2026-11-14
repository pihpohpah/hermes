package models

import (
	"strconv"
	"strings"
	"time"
)

// IdentityProviderTelegram identifies rows in user_identities that came from
// the Telegram Login Widget. More providers join the same table later.
const IdentityProviderTelegram = "telegram"

const (
	identityProviderMaxLength = 32
	identitySubjectMaxLength  = 128
)

// UserIdentity is an external login bound to a user: one row per provider
// account, so a user can accumulate several ways to sign in. Subject is the
// stable provider-side identifier (for Telegram — the numeric tgId); the phone
// number is deliberately not part of it, the user can add it to the account
// later. Data keeps the provider payload (username, photo url, ...) as-is.
type UserIdentity struct {
	ID        string
	UserID    string
	Provider  string
	Subject   string
	Data      map[string]any
	CreatedAt time.Time
}

// NewUserIdentity validates a binding before it is persisted. userID must
// already exist; provider and subject are trimmed and length-checked.
func NewUserIdentity(userID, provider, subject string, data map[string]any) (UserIdentity, error) {
	if strings.TrimSpace(userID) == "" {
		return UserIdentity{}, Invalid("`user_id` is required")
	}

	provider = strings.TrimSpace(provider)
	if provider == "" || len([]rune(provider)) > identityProviderMaxLength {
		return UserIdentity{}, Invalidf(
			"`provider` length must be between 1 and %d characters, got `%s`",
			identityProviderMaxLength, provider,
		)
	}

	subject = strings.TrimSpace(subject)
	if subject == "" || len([]rune(subject)) > identitySubjectMaxLength {
		return UserIdentity{}, Invalidf(
			"`subject` length must be between 1 and %d characters, got `%s`",
			identitySubjectMaxLength, subject,
		)
	}

	if data == nil {
		data = map[string]any{}
	}

	return UserIdentity{
		UserID:   userID,
		Provider: provider,
		Subject:  subject,
		Data:     data,
	}, nil
}

// TelegramDisplayName builds a profile name from the Telegram payload:
// "First Last" when both are present, otherwise whichever exists, otherwise a
// neutral fallback.
func TelegramDisplayName(data map[string]any) string {
	first, _ := data["first_name"].(string)
	first = strings.TrimSpace(first)
	last, _ := data["last_name"].(string)
	last = strings.TrimSpace(last)
	parts := make([]string, 0, telegramNameParts)
	if first != "" {
		parts = append(parts, first)
	}
	if last != "" {
		parts = append(parts, last)
	}
	if name := strings.Join(parts, " "); name != "" {
		return name
	}

	if username, _ := data["username"].(string); strings.TrimSpace(username) != "" {
		return "@" + strings.TrimSpace(username)
	}

	return "Telegram"
}

// TelegramIDClaims — claims id_token нового OIDC-входа Telegram
// (https://core.telegram.org/bots/telegram-login). Подпись токена, iss, aud и
// exp перепроверяет инфраструктурный верификатор; use case получает уже
// доверенные claims и не верит клиенту ничего сверх них.
type TelegramIDClaims struct {
	Issuer     string `json:"iss"`
	Audience   any    `json:"aud"` // строка или массив строк
	Subject    string `json:"sub"`
	ExpiresAt  int64  `json:"exp"`
	IssuedAt   int64  `json:"iat"`
	ID         int64  `json:"id"`   // числовой tgId — субъект идентичности
	Name       string `json:"name"` // полное имя «First Last»
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
	Username   string `json:"preferred_username"`
	Picture    string `json:"picture"`
	Nonce      string `json:"nonce"`
}

// telegramNameParts — first + last name в отображаемом имени профиля.
const telegramNameParts = 2

// IdentitySubject — стабильный идентификатор привязки: числовой tgId из claim
// id, при его отсутствии — sub. Формат совпадает с тем, что раньше писал
// legacy-виджет, поэтому уже созданные привязки продолжают матчиться.
func (c TelegramIDClaims) IdentitySubject() string {
	if c.ID > 0 {
		return strconv.FormatInt(c.ID, 10)
	}

	return strings.TrimSpace(c.Subject)
}

// IdentityData normalizes the claims for storage in the identity row в форме,
// которую понимает TelegramDisplayName: first_name/last_name/username.
func (c TelegramIDClaims) IdentityData() map[string]any {
	first := strings.TrimSpace(c.GivenName)
	if first == "" {
		// given_name может отсутствовать, а name — полным «First Last»:
		// берём первое слово, чтобы DisplayName не склеивал имя дважды.
		first, _, _ = strings.Cut(strings.TrimSpace(c.Name), " ")
	}

	data := map[string]any{
		"first_name": first,
		"username":   c.Username,
		"auth_date":  c.IssuedAt,
	}
	if last := strings.TrimSpace(c.FamilyName); last != "" {
		data["last_name"] = last
	}
	if c.Picture != "" {
		data["photo_url"] = c.Picture
	}

	return data
}
