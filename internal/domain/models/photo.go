package models

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

// NewUUIDv7 returns an RFC 9562 UUIDv7 for the given instant: the first 48
// bits are unix milliseconds, so rows sort roughly by upload time. Generated
// in the application because the database version cannot be relied on to
// provide uuidv7().
func NewUUIDv7(now time.Time) (string, error) {
	var b [16]byte

	ms := uint64(now.UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)

	if _, err := rand.Read(b[6:]); err != nil {
		return "", fmt.Errorf("generate uuid v7: %w", err)
	}

	b[6] = (b[6] & 0x0f) | 0x70 // version 7
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16],
	), nil
}

// PhotoStatus is the lifecycle of a stored photo file.
type PhotoStatus string

const (
	// PhotoStatusModeration is the state right after upload: the file waits
	// for moderation before it becomes publicly addressable.
	PhotoStatusModeration PhotoStatus = "moderation"
	PhotoStatusActive     PhotoStatus = "active"
	PhotoStatusRejected   PhotoStatus = "rejected"
	// PhotoStatusDeleted is a soft delete: the row stays, the file is gone.
	PhotoStatusDeleted PhotoStatus = "deleted"
)

func NewPhotoStatus(raw string) (PhotoStatus, error) {
	status := PhotoStatus(strings.TrimSpace(raw))
	switch status {
	case PhotoStatusModeration, PhotoStatusActive, PhotoStatusRejected, PhotoStatusDeleted:
		return status, nil
	default:
		return "", Invalid(fmt.Sprintf(
			"unexpected photo status `%s`, expected: moderation, active, rejected, deleted", raw,
		))
	}
}

// StoredPhoto is one uploaded photo asset. The file itself lives outside the
// database at StoragePath once storage is wired; the row carries moderation
// state and attribution metadata for the catalog card.
type StoredPhoto struct {
	ID           string
	OwnerID      *string
	Status       PhotoStatus
	StoragePath  string
	OriginalName string
	ContentType  string
	SizeBytes    int64
	Width        *int
	Height       *int
	SHA256       string
	Alt          string
	Author       string
	License      string
	LicenseURL   string
	SourceURL    string
	PlaceID      *string
	SubmissionID *string
	ModeratedBy  *string
	ModeratedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CreatePhotoInput is the parsed upload: the uploader plus everything the
// multipart headers tell us. StoragePath stays empty until storage lands.
type CreatePhotoInput struct {
	OwnerID      *string
	OriginalName string
	ContentType  string
	SizeBytes    int64
	SHA256       string
}

func (in CreatePhotoInput) Validate() error {
	if in.SizeBytes < 0 {
		return Invalid("`size_bytes` must not be negative")
	}
	if in.SizeBytes == 0 {
		return Invalid("uploaded file is empty")
	}

	return nil
}
