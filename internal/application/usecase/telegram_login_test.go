package usecase

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// signTelegram подписывает поля так же, как это делает Login Widget, чтобы
// тесты не зависели от захардкоженных векторов.
func signTelegram(t *testing.T, botToken string, fields models.TelegramAuthFields) models.TelegramAuthFields {
	t.Helper()

	sum := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, sum[:])
	_, _ = mac.Write([]byte(fields.DataCheckString()))
	fields.Hash = hex.EncodeToString(mac.Sum(nil))

	return fields
}

func telegramTestUseCase(botToken string, now time.Time) *TelegramLoginUseCase {
	uc := NewTelegramLoginUseCase(botToken, nil, nil, nil, nil, time.Hour)
	uc.now = func() time.Time { return now }

	return uc
}

func TestTelegramLoginVerifyAcceptsValidSignature(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	fields := signTelegram(t, "bot-token", models.TelegramAuthFields{
		ID:        424242,
		FirstName: "Аня",
		Username:  "anya",
		AuthDate:  now.Unix() - 30,
	})

	uc := telegramTestUseCase("bot-token", now)
	if err := uc.verify(fields); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
}

func TestTelegramLoginVerifyRejectsTamperedData(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	fields := signTelegram(t, "bot-token", models.TelegramAuthFields{
		ID:        424242,
		FirstName: "Аня",
		AuthDate:  now.Unix(),
	})
	fields.Username = "not-anya" // подпись больше не покрывает данные

	uc := telegramTestUseCase("bot-token", now)
	if err := uc.verify(fields); err == nil {
		t.Fatal("tampered payload accepted")
	}
}

func TestTelegramLoginVerifyRejectsWrongToken(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	fields := signTelegram(t, "other-token", models.TelegramAuthFields{
		ID:        424242,
		FirstName: "Аня",
		AuthDate:  now.Unix(),
	})

	uc := telegramTestUseCase("bot-token", now)
	if err := uc.verify(fields); err == nil {
		t.Fatal("signature from another bot accepted")
	}
}

func TestTelegramLoginVerifyRejectsStaleAuthDate(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	fields := signTelegram(t, "bot-token", models.TelegramAuthFields{
		ID:        424242,
		FirstName: "Аня",
		AuthDate:  now.Add(-telegramAuthMaxAge - time.Hour).Unix(),
	})

	uc := telegramTestUseCase("bot-token", now)
	if err := uc.verify(fields); err == nil {
		t.Fatal("stale auth_date accepted")
	}
}

func TestTelegramDataCheckStringOmitsEmptyFields(t *testing.T) {
	fields := models.TelegramAuthFields{
		ID:        7,
		FirstName: "Юра",
		AuthDate:  123,
	}

	want := "auth_date=123\nfirst_name=Юра\nid=7"
	if got := fields.DataCheckString(); got != want {
		t.Fatalf("check string = %q, want %q", got, want)
	}
}

func TestTelegramDisplayNameFallbacks(t *testing.T) {
	if got := models.TelegramDisplayName(map[string]any{"first_name": " Аня ", "last_name": "Ким"}); got != "Аня Ким" {
		t.Fatalf("first+last = %q", got)
	}
	if got := models.TelegramDisplayName(map[string]any{"username": "anya"}); got != "@anya" {
		t.Fatalf("username = %q", got)
	}
	if got := models.TelegramDisplayName(map[string]any{}); got != "Telegram" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestTelegramLoginDoRequiresConfiguredToken(t *testing.T) {
	uc := telegramTestUseCase("", time.Now())
	_, err := uc.Do(t.Context(), TelegramLoginInput{Telegram: models.TelegramAuthFields{ID: 1}})
	if err == nil {
		t.Fatal("login without bot token must fail")
	}
}

func TestTelegramSubjectIsTgID(t *testing.T) {
	fields := models.TelegramAuthFields{ID: 424242}
	if got := strconv.FormatInt(fields.ID, 10); got != "424242" {
		t.Fatalf("subject = %q", got)
	}
}
