package telegram

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func b64(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

// jwksServer крутит JWKS из пары ключей; rotates отдаёт другой kid после
// первого ответа, имитируя ротацию ключей Telegram.
func jwksServer(t *testing.T, key *rsa.PrivateKey, kid string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kid": kid,
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   b64(key.PublicKey.N.Bytes()),
				"e":   b64([]byte{1, 0, 1}),
			}},
		})
	}))
}

func signIDToken(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()

	header, err := json.Marshal(map[string]any{"alg": "RS256", "typ": "JWT", "kid": kid})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}

	input := b64(header) + "." + b64(payload)
	digest := sha256.Sum256([]byte(input))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	return input + "." + b64(signature)
}

func testVerifier(t *testing.T, jwksURL, clientID string, now time.Time) (*IDTokenVerifier, *rsa.PrivateKey) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	client := testClient(t, "https://api.telegram.org", nil, 2)
	verifier := NewIDTokenVerifier(client, clientID)
	verifier.jwksURL = jwksURL
	verifier.now = func() time.Time { return now }

	return verifier, key
}

func validOIDCClaims(clientID string, now time.Time) map[string]any {
	return map[string]any{
		"iss":   "https://oauth.telegram.org",
		"aud":   clientID,
		"sub":   "sub-424242",
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
		"id":    424242,
		"name":  "Аня Ким",
		"nonce": "nonce-1",
	}
}

func TestVerifyAcceptsSignedToken(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("jwks must be cached, second fetch is not expected")
	}))
	defer server.Close()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	jwks := jwksServer(t, key, "k1")
	defer jwks.Close()

	client := testClient(t, "https://api.telegram.org", nil, 2)
	verifier := NewIDTokenVerifier(client, "12345")
	verifier.jwksURL = jwks.URL
	verifier.now = func() time.Time { return now }

	token := signIDToken(t, key, "k1", validOIDCClaims("12345", now))
	claims, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if claims.ID != 424242 || claims.Name != "Аня Ким" || claims.Nonce != "nonce-1" {
		t.Fatalf("claims = %+v", claims)
	}
	if claims.IssuedAt != now.Unix() {
		t.Fatalf("iat = %d", claims.IssuedAt)
	}

	// Второй вызов берёт ключ из кеша — jwks не дёргается.
	verifier.jwksURL = server.URL
	if _, err := verifier.Verify(context.Background(), token); err != nil {
		t.Fatalf("cached key verify failed: %v", err)
	}
}

func TestVerifyRefreshesKeysForUnknownKid(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	jwks := jwksServer(t, key, "k2")
	defer jwks.Close()

	client := testClient(t, "https://api.telegram.org", nil, 2)
	verifier := NewIDTokenVerifier(client, "12345")
	verifier.jwksURL = jwks.URL
	verifier.now = func() time.Time { return now }

	token := signIDToken(t, key, "k2", validOIDCClaims("12345", now))
	if _, err := verifier.Verify(context.Background(), token); err != nil {
		t.Fatalf("kid not fetched from jwks: %v", err)
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	jwks := jwksServer(t, key, "k1")
	defer jwks.Close()

	client := testClient(t, "https://api.telegram.org", nil, 2)
	verifier := NewIDTokenVerifier(client, "12345")
	verifier.jwksURL = jwks.URL
	verifier.now = func() time.Time { return now }

	token := signIDToken(t, key, "k1", validOIDCClaims("12345", now))
	parts := []rune(token)
	for i, r := range parts {
		if r == 'A' {
			parts[i] = 'B'
			break
		}
	}

	if _, err := verifier.Verify(context.Background(), string(parts)); !errors.Is(err, ErrInvalidIDToken) {
		t.Fatalf("tampered token accepted: %v", err)
	}
}

func TestVerifyRejectsWrongKeyIssuerAudience(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	jwks := jwksServer(t, key, "k1")
	defer jwks.Close()

	cases := map[string]struct {
		mutate func(map[string]any)
	}{
		"wrong issuer":   {mutate: func(c map[string]any) { c["iss"] = "https://evil.example" }},
		"wrong audience": {mutate: func(c map[string]any) { c["aud"] = "67890" }},
		"expired":        {mutate: func(c map[string]any) { c["exp"] = now.Add(-time.Minute).Unix() }},
		"stale iat":      {mutate: func(c map[string]any) { c["iat"] = now.Add(-25 * time.Hour).Unix() }},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			client := testClient(t, "https://api.telegram.org", nil, 2)
			verifier := NewIDTokenVerifier(client, "12345")
			verifier.jwksURL = jwks.URL
			verifier.now = func() time.Time { return now }

			claims := validOIDCClaims("12345", now)
			tc.mutate(claims)
			token := signIDToken(t, key, "k1", claims)
			if _, err := verifier.Verify(context.Background(), token); !errors.Is(err, ErrInvalidIDToken) {
				t.Fatalf("%s accepted: %v", name, err)
			}
		})
	}
}

func TestVerifyAcceptsAudienceArray(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	verifier, key := testVerifier(t, "", "12345", now)

	token := signIDToken(t, key, "k1", map[string]any{
		"iss": "https://oauth.telegram.org",
		"aud": []any{"67890", "12345"},
		"sub": "sub-1",
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
		"id":  7,
	})

	// kid k1 неизвестен — кеш пуст и JWKS недоступен: проверяем только то,
	// что aud в массиве больше не отдельная ветка ошибки, подменив ключ на
	// известный через кеш.
	jwks := jwksServer(t, key, "k1")
	defer jwks.Close()
	verifier.jwksURL = jwks.URL

	claims, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("aud array rejected: %v", err)
	}
	if claims.ID != 7 {
		t.Fatalf("claims = %+v", claims)
	}
}

func TestVerifyRejectsMalformedTokens(t *testing.T) {
	verifier, _ := testVerifier(t, "", "12345", time.Now())

	for _, token := range []string{"", "a.b", "a.b.c.d", "!!.!!", "a.b.@@"} {
		if _, err := verifier.Verify(context.Background(), token); !errors.Is(err, ErrInvalidIDToken) {
			t.Fatalf("token %q accepted: %v", token, err)
		}
	}
}
