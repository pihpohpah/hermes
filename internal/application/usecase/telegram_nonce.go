package usecase

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"sync"
	"time"
)

// TelegramNonceIssuer выдаёт и проверяет nonce для Telegram OIDC-входа
// (https://core.telegram.org/bots/telegram-login: nonce «to prevent replay
// attacks»). Формат — base64url(json{"exp","r"}) + "." + base64url(HMAC-SHA256
// ключа): состояние хранить не нужно, подделка nonce невозможна без ключа,
// а вместе с exp токен нельзя воспроизвести после окна входа.
type TelegramNonceIssuer struct {
	key [sha256.Size]byte
	ttl time.Duration
	now func() time.Time
	rnd io.Reader

	mu sync.Mutex
}

// NewTelegramNonceIssuer держит ключ входа в квадрате: основная стойкость —
// в подписи Telegram по JWKS, nonce лишь привязывает токен к свежему
// серверному вызову, поэтому ключ — это sha256(secret), а secret берётся из
// уже существующих секретов (bot token, при его отсутствии — client_id).
func NewTelegramNonceIssuer(secret string, ttl time.Duration) *TelegramNonceIssuer {
	return &TelegramNonceIssuer{
		key: sha256.Sum256([]byte("hermes/telegram-login/nonce\x00" + secret)),
		ttl: ttl,
		now: time.Now,
		rnd: rand.Reader,
	}
}

type telegramNoncePayload struct {
	Exp int64  `json:"exp"`
	R   string `json:"r"`
}

// Issue возвращает новый nonce: случайная часть уникальна, exp ограничивает
// окно, в которое токен с этим nonce принимается.
func (i *TelegramNonceIssuer) Issue() (string, error) {
	var random [16]byte
	if _, err := io.ReadFull(i.rnd, random[:]); err != nil {
		return "", err
	}

	payload, err := json.Marshal(telegramNoncePayload{
		Exp: i.now().Add(i.ttl).Unix(),
		R:   base64.RawURLEncoding.EncodeToString(random[:]),
	})
	if err != nil {
		return "", err
	}

	encoded := make([]byte, base64.RawURLEncoding.EncodedLen(len(payload)))
	base64.RawURLEncoding.Encode(encoded, payload)

	return string(encoded) + "." + i.sign(encoded), nil
}

// Valid проверяет подпись и срок nonce; сравнение подписи — constant time.
func (i *TelegramNonceIssuer) Valid(nonce string) bool {
	encoded, sig, ok := cutLast(nonce)
	if !ok {
		return false
	}

	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}

	var decoded telegramNoncePayload
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded.R == "" {
		return false
	}

	if exp := time.Unix(decoded.Exp, 0); !i.now().Before(exp) {
		return false
	}

	expected := i.sign([]byte(encoded))

	return hmac.Equal([]byte(sig), []byte(expected))
}

// sign подписывает сырой payload nonce; ключ живёт в mu-значении struct.
func (i *TelegramNonceIssuer) sign(payload []byte) string {
	i.mu.Lock()
	mac := hmac.New(sha256.New, i.key[:])
	i.mu.Unlock()
	_, _ = mac.Write(payload)

	encoded := make([]byte, base64.RawURLEncoding.EncodedLen(sha256.Size))
	base64.RawURLEncoding.Encode(encoded, mac.Sum(nil))

	return string(encoded)
}

// cutLast делит строку по последней точке: payload nonce base64url-формата,
// точек внутри не быть.
func cutLast(s string) (head, tail string, ok bool) {
	for idx := len(s) - 1; idx >= 0; idx-- {
		if s[idx] == '.' {
			return s[:idx], s[idx+1:], true
		}
	}

	return "", "", false
}
