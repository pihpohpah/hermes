package models

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Opaque token length in bytes before base64url encoding. 256 bits of entropy
// means the token space is large enough that guessing is not a concern.
const opaqueTokenBytes = 32

// UserSession is a server-side session. Only hashes of the opaque tokens ever
// reach the database, so a leaked table yields no usable cookies or CSRF
// secrets.
type UserSession struct {
	ID        string
	UserID    string
	TokenHash string
	CSRFHash  string
	CreatedAt time.Time
	ExpiresAt time.Time
	// RevokedAt is nil while the session is valid; a revoked session never
	// authenticates again even before ExpiresAt.
	RevokedAt *time.Time
}

// NewOpaqueToken returns a random opaque token and the hex SHA-256 of it.
// The token goes to the client (cookie / response body), the hash is stored.
func NewOpaqueToken() (token string, hash string, err error) {
	raw := make([]byte, opaqueTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}

	token = base64.RawURLEncoding.EncodeToString(raw)

	return token, HashToken(token), nil
}

// HashToken derives the stored form of an opaque token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}

// ActiveAt reports whether the session can still authenticate requests.
func (s UserSession) ActiveAt(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

// CSRFMatches compares a token from the X-CSRF-Token header with the stored
// hash in constant time.
func (s UserSession) CSRFMatches(token string) bool {
	given := HashToken(token)

	return subtle.ConstantTimeCompare([]byte(given), []byte(s.CSRFHash)) == 1
}

// NewUserSession builds a session valid for ttl starting at now. The caller
// passes the pre-hashed tokens produced together with the opaque values.
func NewUserSession(userID, tokenHash, csrfHash string, now time.Time, ttl time.Duration) (UserSession, error) {
	if userID == "" {
		return UserSession{}, Invalid("`user_id` is required")
	}

	if tokenHash == "" || csrfHash == "" {
		return UserSession{}, errors.New("token hashes must not be empty")
	}

	if ttl <= 0 {
		return UserSession{}, Invalidf("session ttl must be positive, got %s", ttl)
	}

	return UserSession{
		UserID:    userID,
		TokenHash: tokenHash,
		CSRFHash:  csrfHash,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}, nil
}
