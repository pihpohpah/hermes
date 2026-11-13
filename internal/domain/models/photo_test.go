package models_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

func TestNewUUIDv7(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
	}{
		{name: "epoch", now: time.Unix(0, 0)},
		{name: "current time", now: time.Now()},
		{name: "far future", now: time.Date(2300, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id, err := models.NewUUIDv7(tc.now)
			if err != nil {
				t.Fatalf("NewUUIDv7: %v", err)
			}

			parts := strings.Split(id, "-")
			if len(parts) != 5 || len(parts[0]) != 8 || len(parts[1]) != 4 ||
				len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
				t.Fatalf("malformed uuid %q", id)
			}
			if parts[2][0] != '7' {
				t.Errorf("version nibble = %q, want 7", parts[2][0])
			}
			if parts[3][0] != '8' && parts[3][0] != '9' && parts[3][0] != 'a' && parts[3][0] != 'b' {
				t.Errorf("variant nibble = %q, want 8/9/a/b", parts[3][0])
			}
		})
	}
}

func TestNewUUIDv7SortsByTime(t *testing.T) {
	first, err := models.NewUUIDv7(time.UnixMilli(1000))
	if err != nil {
		t.Fatalf("NewUUIDv7: %v", err)
	}
	second, err := models.NewUUIDv7(time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("NewUUIDv7: %v", err)
	}

	if second <= first {
		t.Errorf("uuid of a later instant must sort after: %q <= %q", second, first)
	}
}

func TestCreatePhotoInputValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*models.CreatePhotoInput)
		wantErr error
	}{
		{name: "non-empty upload is valid", mutate: func(*models.CreatePhotoInput) {}},
		{name: "empty file rejected", mutate: func(in *models.CreatePhotoInput) {
			in.SizeBytes = 0
		}, wantErr: models.ErrInvalidArgument},
		{name: "negative size rejected", mutate: func(in *models.CreatePhotoInput) {
			in.SizeBytes = -1
		}, wantErr: models.ErrInvalidArgument},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := models.CreatePhotoInput{
				OriginalName: "photo.jpg",
				ContentType:  "image/jpeg",
				SizeBytes:    1024,
				SHA256:       "abc",
			}
			tc.mutate(&in)

			err := in.Validate()
			if tc.wantErr == nil && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
