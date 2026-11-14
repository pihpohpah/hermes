package telegram

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// Endpoint'ы Telegram OIDC (https://core.telegram.org/bots/telegram-login#openid-connect).
const (
	defaultIssuer  = "https://oauth.telegram.org"
	defaultJWKSURL = "https://oauth.telegram.org/.well-known/jwks.json"
)

// ErrInvalidIDToken — подпись id_token не бьётся или claims противоречат
// нашему client_id: ретраить или настраивать конфиг надо, запрос не поможет.
var ErrInvalidIDToken = errors.New("invalid telegram id_token")

// idTokenMaxAge отсекает подписанные Telegram токены, которым больше суток:
// даже при корректном exp перехваченный токен не должен давать вход вечно.
const idTokenMaxAge = 24 * time.Hour

// IDTokenVerifier проверяет id_token нового OIDC-входа Telegram: подпись
// RS256 по публичным ключам из JWKS (с кешем и перечитыванием при новом kid),
// затем iss/aud/exp. Токен всегда разбирается только после проверки подписи.
type IDTokenVerifier struct {
	client   *Client
	clientID string
	issuer   string
	jwksURL  string
	now      func() time.Time

	mu   sync.Mutex
	keys map[string]*rsa.PublicKey // kid → публичный ключ
}

func NewIDTokenVerifier(client *Client, clientID string) *IDTokenVerifier {
	return &IDTokenVerifier{
		client:   client,
		clientID: strings.TrimSpace(clientID),
		issuer:   defaultIssuer,
		jwksURL:  defaultJWKSURL,
		now:      time.Now,
		keys:     map[string]*rsa.PublicKey{},
	}
}

// Verify возвращает доверенные claims id_token. Проверяются подпись RS256,
// iss, aud (строка или массив) и свежесть iat/exp; nonce здесь не проверяется —
// его свежесть и выдачу валидирует use case.
func (v *IDTokenVerifier) Verify(ctx context.Context, rawToken string) (models.TelegramIDClaims, error) {
	var claims models.TelegramIDClaims

	headerSeg, payloadSeg, signingInput, signature, err := splitToken(rawToken)
	if err != nil {
		return claims, err
	}

	var hdr struct {
		Alg string `json:"alg"`
		KID string `json:"kid"`
	}
	if err := json.Unmarshal(decodeSegment(headerSeg), &hdr); err != nil || hdr.Alg != "RS256" || hdr.KID == "" {
		return claims, fmt.Errorf("%w: unsupported token header", ErrInvalidIDToken)
	}

	key, err := v.key(ctx, hdr.KID)
	if err != nil {
		return claims, err
	}

	digest := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return claims, fmt.Errorf("%w: signature", ErrInvalidIDToken)
	}

	if err := json.Unmarshal(decodeSegment(payloadSeg), &claims); err != nil {
		return claims, fmt.Errorf("%w: claims: %v", ErrInvalidIDToken, err)
	}

	if claims.Issuer != v.issuer {
		return claims, fmt.Errorf("%w: iss %q", ErrInvalidIDToken, claims.Issuer)
	}
	if !audienceMatches(claims.Audience, v.clientID) {
		return claims, fmt.Errorf("%w: aud %v", ErrInvalidIDToken, claims.Audience)
	}
	if claims.ExpiresAt <= v.now().Unix() {
		return claims, fmt.Errorf("%w: token expired", ErrInvalidIDToken)
	}
	if claims.IssuedAt > 0 {
		age := v.now().Sub(time.Unix(claims.IssuedAt, 0))
		if age > idTokenMaxAge || age < -time.Minute {
			return claims, fmt.Errorf("%w: token issued %s ago", ErrInvalidIDToken, age.Truncate(time.Second))
		}
	}

	return claims, nil
}

// key возвращает публичный ключ по kid: из кеша, при промахе — после
// перечитывания JWKS (Telegram ротирует ключи без уведомления).
func (v *IDTokenVerifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	cached := v.keys[kid]
	v.mu.Unlock()
	if cached != nil {
		return cached, nil
	}

	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	if key := v.keys[kid]; key != nil {
		return key, nil
	}

	return nil, fmt.Errorf("%w: unknown kid %q", ErrInvalidIDToken, kid)
}

func (v *IDTokenVerifier) refreshKeys(ctx context.Context) error {
	raw, err := v.client.GetJSON(ctx, v.jwksURL)
	if err != nil {
		return fmt.Errorf("fetch telegram jwks: %w", err)
	}

	var jwks struct {
		Keys []struct {
			KID string `json:"kid"`
			Kty string `json:"kty"`
			Alg string `json:"alg"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(raw, &jwks); err != nil {
		return fmt.Errorf("decode telegram jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, jwk := range jwks.Keys {
		if jwk.Kty != "RSA" || jwk.KID == "" {
			continue
		}
		key, err := rsaPublicKey(jwk.N, jwk.E)
		if err != nil {
			return fmt.Errorf("decode telegram jwk %q: %w", jwk.KID, err)
		}
		keys[jwk.KID] = key
	}
	if len(keys) == 0 {
		return errors.New("telegram jwks has no usable RSA keys")
	}

	v.mu.Lock()
	v.keys = keys
	v.mu.Unlock()

	return nil
}

// splitToken режет JWT на сырые base64url-сегменты header/payload, signing
// input (header.payload — то, что покрывает подпись) и саму подпись,
// ничего не декодируя заранее.
func splitToken(token string) (headerSeg, payloadSeg, signingInput string, signature []byte, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", "", "", nil, fmt.Errorf("%w: malformed jwt", ErrInvalidIDToken)
	}

	signature, err = base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", "", "", nil, fmt.Errorf("%w: signature encoding", ErrInvalidIDToken)
	}

	if len(decodeSegment(parts[0])) == 0 || len(decodeSegment(parts[1])) == 0 {
		return "", "", "", nil, fmt.Errorf("%w: malformed jwt segments", ErrInvalidIDToken)
	}

	return parts[0], parts[1], parts[0] + "." + parts[1], signature, nil
}

func decodeSegment(segment string) []byte {
	decoded, _ := base64.RawURLEncoding.DecodeString(segment)

	return decoded
}

func rsaPublicKey(n, e string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(n)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(e)
	if err != nil {
		return nil, err
	}

	exponent := new(big.Int).SetBytes(eBytes).Uint64()
	if exponent == 0 {
		exponent = 65537 // "AQAB" всегда так, но пустой e не должен молча стать 0
	}

	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: int(exponent)}, nil
}

// audienceMatches принимает aud и строкой, и массивом строк — Telegram OIDC
// отдаёт строку, но OIDC-библиотеки привыкли к обоим вариантам.
func audienceMatches(audience any, clientID string) bool {
	switch aud := audience.(type) {
	case string:
		return aud == clientID
	case []any:
		for _, item := range aud {
			if value, ok := item.(string); ok && value == clientID {
				return true
			}
		}
	}

	return false
}
