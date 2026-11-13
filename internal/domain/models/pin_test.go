package models_test

import (
	"testing"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantLen int
	}{
		{name: "ascii lowercase", raw: "Tokarevsky Lighthouse", want: "tokarevsky-lighthouse"},
		{name: "russian", raw: "Маяк Токаревского", want: "mayak-tokarevskogo"},
		{name: "mixed punctuation", raw: "Сопка «Бурачка»!", want: "sopka-burachka"},
		{name: "digits kept", raw: "Пляж № 9", want: "plyazh-9"},
		{name: "collapse separators", raw: "a  --  b__c", want: "a-b-c"},
		{name: "empty falls back to hash", raw: "🎉", wantLen: 9},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := models.Slugify(tc.raw)
			if tc.wantLen > 0 {
				if len(got) != tc.wantLen {
					t.Fatalf("Slugify(%q) length = %d, want %d (%q)", tc.raw, len(got), tc.wantLen, got)
				}

				return
			}
			if got != tc.want {
				t.Fatalf("Slugify(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestNewPinBoundsRejectsInverted(t *testing.T) {
	if _, err := models.NewPinBounds(10, 20, 10, 30); err == nil {
		t.Fatal("expected error for minLon == maxLon")
	}
	if _, err := models.NewPinBounds(10, 30, 20, 20); err == nil {
		t.Fatal("expected error for minLat == maxLat")
	}
	if _, err := models.NewPinBounds(131.8, 43.0, 132.0, 43.2); err != nil {
		t.Fatalf("valid bounds rejected: %v", err)
	}
}

func TestCreatePinInputValidate(t *testing.T) {
	base := models.CreatePinInput{
		Kind:     models.PinKindSport,
		Title:    "Футбол на Спортивной",
		Location: models.Point{Lat: 43.119, Lon: 131.874},
		StartsAt: time.Now().Add(30 * time.Minute),
		EndsAt:   time.Now().Add(2 * time.Hour),
		AuthorID: "00000000-0000-0000-0000-000000000001",
	}

	if err := base.Validate(12 * time.Hour); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}

	tooLong := base
	tooLong.Title = string(make([]byte, 141))
	if err := tooLong.Validate(12 * time.Hour); err == nil {
		t.Fatal("expected error for a 141-character title")
	}

	endsBeforeStart := base
	endsBeforeStart.EndsAt = endsBeforeStart.StartsAt.Add(-time.Minute)
	if err := endsBeforeStart.Validate(12 * time.Hour); err == nil {
		t.Fatal("expected error when ends_at is before starts_at")
	}

	tooLongDuration := base
	tooLongDuration.EndsAt = tooLongDuration.StartsAt.Add(24 * time.Hour)
	if err := tooLongDuration.Validate(12 * time.Hour); err == nil {
		t.Fatal("expected error for a pin longer than the max duration")
	}

	badNeed := base
	need := 101
	badNeed.Need = &need
	if err := badNeed.Validate(12 * time.Hour); err == nil {
		t.Fatal("expected error for need above 100")
	}
}
