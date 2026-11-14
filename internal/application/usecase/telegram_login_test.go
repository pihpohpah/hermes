package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// fakeVerifier имитирует инфраструктурную проверку id_token: тесты use case
// смотрят на то, что происходит с уже проверенными claims и с провалом
// верификации, а не на криптографию JWKS.
type fakeVerifier struct {
	claims models.TelegramIDClaims
	err    error
}

func (f fakeVerifier) Verify(_ context.Context, _ string) (models.TelegramIDClaims, error) {
	return f.claims, f.err
}

func telegramTestUseCase(clientID string, verifier idTokenVerifier, now time.Time) *TelegramLoginUseCase {
	nonces := NewTelegramNonceIssuer("test-secret", 15*time.Minute)
	nonces.now = func() time.Time { return now }

	uc := NewTelegramLoginUseCase(clientID, verifier, nonces, nil, nil, nil, nil, time.Hour)
	uc.now = func() time.Time { return now }

	return uc
}

func validTestClaims(nonce string) models.TelegramIDClaims {
	return models.TelegramIDClaims{
		Issuer:    "https://oauth.telegram.org",
		Audience:  "12345",
		Subject:   "sub-424242",
		ExpiresAt: 1_760_000_600,
		IssuedAt:  1_760_000_000,
		ID:        424242,
		Name:      "Аня Ким",
		GivenName: "Аня",
		Nonce:     nonce,
	}
}

func TestTelegramLoginVerifyAcceptsValidToken(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	nonces := NewTelegramNonceIssuer("test-secret", 15*time.Minute)
	nonce, err := nonces.Issue()
	if err != nil {
		t.Fatalf("issue nonce: %v", err)
	}

	uc := telegramTestUseCase("12345", fakeVerifier{claims: validTestClaims(nonce)}, now)
	claims, err := uc.verify(t.Context(), "id-token")
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if claims.ID != 424242 {
		t.Fatalf("claims = %+v", claims)
	}
}

func TestTelegramLoginVerifyRejectsBadSignature(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	uc := telegramTestUseCase(
		"12345",
		fakeVerifier{err: errors.New("invalid telegram id_token: signature")},
		now,
	)

	if _, err := uc.verify(t.Context(), "forged-token"); err == nil {
		t.Fatal("token with bad signature accepted")
	}
}

func TestTelegramLoginVerifyRejectsTokenWithoutNonce(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	uc := telegramTestUseCase("12345", fakeVerifier{claims: validTestClaims("")}, now)

	if _, err := uc.verify(t.Context(), "id-token"); err == nil {
		t.Fatal("token without nonce accepted")
	}
}

func TestTelegramLoginVerifyRejectsForeignNonce(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	uc := telegramTestUseCase("12345", fakeVerifier{claims: validTestClaims("not-our-nonce")}, now)

	if _, err := uc.verify(t.Context(), "id-token"); err == nil {
		t.Fatal("token with nonce from another server accepted")
	}
}

func TestTelegramLoginVerifyRejectsExpiredNonce(t *testing.T) {
	issuedAt := time.Unix(1_760_000_000, 0)
	nonces := NewTelegramNonceIssuer("test-secret", 15*time.Minute)
	nonces.now = func() time.Time { return issuedAt }
	nonce, err := nonces.Issue()
	if err != nil {
		t.Fatalf("issue nonce: %v", err)
	}

	// Проверяем уже после окна жизни nonce.
	uc := telegramTestUseCase("12345", fakeVerifier{claims: validTestClaims(nonce)}, issuedAt.Add(16*time.Minute))
	if _, err := uc.verify(t.Context(), "id-token"); err == nil {
		t.Fatal("token with expired nonce accepted")
	}
}

func TestTelegramLoginDoRequiresConfiguredClientID(t *testing.T) {
	uc := telegramTestUseCase("", fakeVerifier{claims: validTestClaims("x")}, time.Now())
	if _, err := uc.Do(t.Context(), TelegramLoginInput{IDToken: "token"}); err == nil {
		t.Fatal("login without client id must fail")
	}
}

func TestTelegramNonceRoundtrip(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	nonces := NewTelegramNonceIssuer("test-secret", 15*time.Minute)
	nonces.now = func() time.Time { return now }

	nonce, err := nonces.Issue()
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !nonces.Valid(nonce) {
		t.Fatal("fresh own nonce rejected")
	}

	other := NewTelegramNonceIssuer("other-secret", 15*time.Minute)
	other.now = nonces.now
	if other.Valid(nonce) {
		t.Fatal("nonce accepted with another key")
	}

	if nonces.Valid(nonce + "tampered") {
		t.Fatal("tampered nonce accepted")
	}
	if nonces.Valid("no-dot") {
		t.Fatal("malformed nonce accepted")
	}

	nonces.now = func() time.Time { return now.Add(16 * time.Minute) }
	if nonces.Valid(nonce) {
		t.Fatal("expired nonce accepted")
	}
}

func TestTelegramIdentitySubjectPrefersNumericID(t *testing.T) {
	claims := models.TelegramIDClaims{ID: 424242, Subject: "sub-424242"}
	if got := claims.IdentitySubject(); got != "424242" {
		t.Fatalf("subject = %q", got)
	}

	claims.ID = 0
	if got := claims.IdentitySubject(); got != "sub-424242" {
		t.Fatalf("fallback subject = %q", got)
	}
}

func TestTelegramIdentityDataMapsOIDCClaims(t *testing.T) {
	claims := models.TelegramIDClaims{
		Name:       "Аня Ким",
		GivenName:  "Аня",
		FamilyName: "Ким",
		Username:   "anya",
		Picture:    "https://cdn4.telesco.pe/anya.jpg",
	}
	data := claims.IdentityData()
	if data["first_name"] != "Аня" || data["last_name"] != "Ким" || data["username"] != "anya" {
		t.Fatalf("data = %v", data)
	}
	if data["photo_url"] != "https://cdn4.telesco.pe/anya.jpg" {
		t.Fatalf("photo = %v", data["photo_url"])
	}

	// given_name отсутствует — имя берётся из первого слова name.
	claims = models.TelegramIDClaims{Name: "Юра Смирнов", Username: "yura"}
	data = claims.IdentityData()
	if data["first_name"] != "Юра" {
		t.Fatalf("first from name = %v", data["first_name"])
	}
	if _, hasLast := data["last_name"]; hasLast {
		t.Fatalf("last_name must be absent, got %v", data["last_name"])
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
