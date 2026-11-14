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

// TelegramAuthFields is the raw payload of the Telegram Login Widget callback
// (https://core.telegram.org/widgets/login). Hash is the HMAC-SHA256 of the
// data-check string with the bot-token key; the use case re-verifies it and
// never trusts the client beyond that.
type TelegramAuthFields struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	PhotoURL  string `json:"photo_url"`
	AuthDate  int64  `json:"auth_date"`
	Hash      string `json:"hash"`
}

// dataCheckPairs — максимум полей виджета, влияющих на подпись.
const dataCheckPairs = 6

// telegramNameParts — first + last name в отображаемом имени профиля.
const telegramNameParts = 2

// DataCheckString is the canonical string Telegram signs: "key=value" pairs
// for every field except hash, sorted by key, joined with newlines. Only
// present fields take part — the widget omits empty ones.
func (f TelegramAuthFields) DataCheckString() string {
	pairs := make([]string, 0, dataCheckPairs)
	add := func(key, value string) {
		if value != "" {
			pairs = append(pairs, key+"="+value)
		}
	}

	// Пары должны быть отсортированы по ключу: auth_date, first_name, id,
	// last_name, photo_url, username.
	add("auth_date", strconv.FormatInt(f.AuthDate, 10))
	add("first_name", f.FirstName)
	add("id", strconv.FormatInt(f.ID, 10))
	add("last_name", f.LastName)
	add("photo_url", f.PhotoURL)
	add("username", f.Username)

	return strings.Join(pairs, "\n")
}

// IdentityData normalizes the payload for storage in the identity row.
func (f TelegramAuthFields) IdentityData() map[string]any {
	data := map[string]any{
		"first_name": f.FirstName,
		"username":   f.Username,
		"auth_date":  f.AuthDate,
	}
	if f.LastName != "" {
		data["last_name"] = f.LastName
	}
	if f.PhotoURL != "" {
		data["photo_url"] = f.PhotoURL
	}

	return data
}
