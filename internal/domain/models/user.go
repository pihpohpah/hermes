package models

import (
	"strings"
	"time"
)

// DefaultTimezone is assigned to a user who did not pick one. The product is
// built around Vladivostok, so its IANA zone is the default.
const DefaultTimezone = "Asia/Vladivostok"

const (
	displayNameMinLength = 1
	displayNameMaxLength = 100
)

// User is a minimal guest profile: a stable identity for trips and reports.
// There is deliberately no password, email or roles here — those arrive later
// as rows in identities, not as columns on this table.
type User struct {
	ID          string
	DisplayName string
	Timezone    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewDisplayName trims surrounding whitespace and enforces the 1–100 char
// rule on what remains, so "   " is rejected but " Юрий " becomes "Юрий".
func NewDisplayName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	length := len([]rune(name))

	if length < displayNameMinLength || length > displayNameMaxLength {
		return "", Invalidf("`display_name` length must be between %d and %d characters after trimming, got %d",
			displayNameMinLength, displayNameMaxLength, length)
	}

	return name, nil
}

// NewTimezone validates raw against the IANA tz database. An empty value
// falls back to DefaultTimezone.
func NewTimezone(raw string) (string, error) {
	zone := strings.TrimSpace(raw)
	if zone == "" {
		return DefaultTimezone, nil
	}

	if _, err := time.LoadLocation(zone); err != nil {
		return "", Invalidf("`timezone` must be an IANA timezone name, got `%s`", raw)
	}

	return zone, nil
}

// NewUser builds a validated profile. ID stays empty: the database generates
// it, so a user without an ID is a user that was never persisted.
func NewUser(displayName, timezone string) (User, error) {
	name, err := NewDisplayName(displayName)
	if err != nil {
		return User{}, err
	}

	zone, err := NewTimezone(timezone)
	if err != nil {
		return User{}, err
	}

	return User{
		DisplayName: name,
		Timezone:    zone,
	}, nil
}

// IdempotencyRecord pins the outcome of a mutating request to a client key,
// so a retried POST /users with the same key returns the same user instead
// of creating a duplicate. Scope namespaces the key for future endpoints.
type IdempotencyRecord struct {
	Scope              string
	Key                string
	RequestFingerprint string
	UserID             string
	ExpiresAt          time.Time
}
