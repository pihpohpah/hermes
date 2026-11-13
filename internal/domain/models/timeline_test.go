package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

func TestDayWindowUsesAsiaVladivostok(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantStart time.Time
		wantEnd   time.Time
		wantHours int
	}{
		{
			name:      "regular day",
			raw:       "2026-11-13",
			wantStart: time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 13, 14, 0, 0, 0, time.UTC),
			wantHours: 24,
		},
		{
			name:      "month boundary",
			raw:       "2026-12-01",
			wantStart: time.Date(2026, 11, 30, 14, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 12, 1, 14, 0, 0, 0, time.UTC),
			wantHours: 24,
		},
		{
			name:      "leap day",
			raw:       "2028-02-29",
			wantStart: time.Date(2028, 2, 28, 14, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2028, 2, 29, 14, 0, 0, 0, time.UTC),
			wantHours: 24,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			window, err := models.ParseDayWindow(tc.raw)
			if err != nil {
				t.Fatalf("parse %s: %v", tc.raw, err)
			}
			if !window.Start.Equal(tc.wantStart) {
				t.Fatalf("start = %s, want %s", window.Start, tc.wantStart)
			}
			if !window.End.Equal(tc.wantEnd) {
				t.Fatalf("end = %s, want %s", window.End, tc.wantEnd)
			}
			if window.Hours() != tc.wantHours {
				t.Fatalf("hours = %d, want %d", window.Hours(), tc.wantHours)
			}
			if window.Date() != tc.raw {
				t.Fatalf("date = %s, want %s", window.Date(), tc.raw)
			}
		})
	}
}

func TestDayWindowIsNotUTCDay(t *testing.T) {
	t.Parallel()

	window, err := models.ParseDayWindow("2026-11-13")
	if err != nil {
		t.Fatalf("parse day: %v", err)
	}

	utcDay := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	if window.Start.Equal(utcDay) {
		t.Fatal("day window must not start at UTC midnight")
	}
	if shift := utcDay.Sub(window.Start); shift != 10*time.Hour {
		t.Fatalf("UTC midnight is shifted by %s from the local day, want 10h", shift)
	}
}

func TestDayWindowContains(t *testing.T) {
	t.Parallel()

	window, err := models.ParseDayWindow("2026-11-13")
	if err != nil {
		t.Fatalf("parse day: %v", err)
	}

	cases := []struct {
		instant time.Time
		want    bool
	}{
		{window.Start, true},
		{window.Start.Add(-time.Nanosecond), false},
		{window.End.Add(-time.Nanosecond), true},
		{window.End, false},
		{time.Date(2026, 11, 13, 2, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 11, 13, 14, 0, 0, 0, time.UTC), false},
	}

	for _, tc := range cases {
		if got := window.Contains(tc.instant); got != tc.want {
			t.Fatalf("Contains(%s) = %v, want %v", tc.instant, got, tc.want)
		}
	}
}

func TestCurrentDayWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 13, 12, 15, 0, 0, time.UTC)
	window, err := models.CurrentDayWindow(now)
	if err != nil {
		t.Fatalf("current day window: %v", err)
	}
	if window.Date() != "2026-11-13" {
		t.Fatalf("date = %s, want 2026-11-13", window.Date())
	}
}

func TestForecastRangeCoversCurrentAndTenDays(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	rng, err := models.NewForecastRange(now)
	if err != nil {
		t.Fatalf("range: %v", err)
	}

	if !rng.Contains(now) {
		t.Fatal("the current moment must be inside the range")
	}
	if days := len(rng.Days()); days != models.ForecastHorizonDays {
		t.Fatalf("horizon days = %d, want %d", days, models.ForecastHorizonDays)
	}
	if rng.Hours() <= 24*models.ForecastHorizonDays/2 {
		t.Fatalf("range is too short: %d hours", rng.Hours())
	}

	lastDay := rng.Days()[len(rng.Days())-1]
	if !rng.Contains(lastDay.Start) {
		t.Fatal("the last horizon day must be inside the range")
	}
	if rng.Contains(lastDay.End) {
		t.Fatal("the range must stop at the end of the last day")
	}
}

func TestForecastRangeKeepsRecentPast(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	rng, err := models.NewForecastRange(now)
	if err != nil {
		t.Fatalf("range: %v", err)
	}

	if !rng.From.Before(now) {
		t.Fatalf("range must start before now, got %s", rng.From)
	}
	if back := now.Sub(rng.From); back < models.ObservedHoursBack*time.Hour {
		t.Fatalf("range looks back only %s, want at least %dh", back, models.ObservedHoursBack)
	}
}

func TestMeasureJSONKeepsUnknownAsNull(t *testing.T) {
	t.Parallel()

	type payload struct {
		Known   models.Measure `json:"known"`
		Zero    models.Measure `json:"zero"`
		Unknown models.Measure `json:"unknown"`
	}

	raw, err := json.Marshal(payload{
		Known:   models.NewMeasure(3.5),
		Zero:    models.NewMeasure(0),
		Unknown: models.MissingMeasure(),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded struct {
		Known   *float64 `json:"known"`
		Zero    *float64 `json:"zero"`
		Unknown *float64 `json:"unknown"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Known == nil || *decoded.Known != 3.5 {
		t.Fatalf("known value lost: %v", decoded.Known)
	}
	if decoded.Zero == nil || *decoded.Zero != 0 {
		t.Fatalf("explicit zero must stay zero, got %v", decoded.Zero)
	}
	if decoded.Unknown != nil {
		t.Fatalf("unknown value must be null, got %v", *decoded.Unknown)
	}
}
