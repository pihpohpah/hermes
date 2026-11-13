package models

import (
	"fmt"
	"time"
	_ "time/tzdata" // keep Asia/Vladivostok resolvable without host tzdata
)

const (
	primoryeTimeZone = "Asia/Vladivostok"
	dateOnlyLayout   = "2006-01-02"
)

func PrimoryeLocation() (*time.Location, error) {
	loc, err := time.LoadLocation(primoryeTimeZone)
	if err != nil {
		return nil, fmt.Errorf("load location `%s`: %w", primoryeTimeZone, err)
	}

	return loc, nil
}

// DayWindow is a calendar day in Asia/Vladivostok expressed as UTC instants.
// Start is inclusive, End is exclusive.
type DayWindow struct {
	Day      time.Time
	Start    time.Time
	End      time.Time
	Location *time.Location
}

func NewDayWindow(year int, month time.Month, day int) (DayWindow, error) {
	loc, err := PrimoryeLocation()
	if err != nil {
		return DayWindow{}, err
	}

	local := time.Date(year, month, day, 0, 0, 0, 0, loc)
	window := DayWindow{
		Day:      local,
		Start:    local.UTC(),
		End:      time.Date(year, month, day+1, 0, 0, 0, 0, loc).UTC(),
		Location: loc,
	}
	if !window.End.After(window.Start) {
		return DayWindow{}, Invalid(fmt.Sprintf("day window `%s` is empty", window.Date()))
	}

	return window, nil
}

// ParseDayWindow interprets YYYY-MM-DD as a calendar date in Asia/Vladivostok.
func ParseDayWindow(raw string) (DayWindow, error) {
	loc, err := PrimoryeLocation()
	if err != nil {
		return DayWindow{}, err
	}

	parsed, parseErr := time.ParseInLocation(dateOnlyLayout, raw, loc)
	if parseErr != nil {
		return DayWindow{}, Invalid(fmt.Sprintf("date `%s` must be in YYYY-MM-DD format", raw))
	}

	return NewDayWindow(parsed.Year(), parsed.Month(), parsed.Day())
}

// CurrentDayWindow returns the calendar day of now in Asia/Vladivostok.
func CurrentDayWindow(now time.Time) (DayWindow, error) {
	loc, err := PrimoryeLocation()
	if err != nil {
		return DayWindow{}, err
	}

	local := now.In(loc)

	return NewDayWindow(local.Year(), local.Month(), local.Day())
}

func (w DayWindow) Date() string {
	return w.Day.Format(dateOnlyLayout)
}

func (w DayWindow) Hours() int {
	return int(w.End.Sub(w.Start) / time.Hour)
}

func (w DayWindow) Contains(instant time.Time) bool {
	return !instant.Before(w.Start) && instant.Before(w.End)
}

// HourStarts returns UTC hour boundaries of the window.
func (w DayWindow) HourStarts() []time.Time {
	hours := w.Hours()
	result := make([]time.Time, 0, hours)
	for offset := range hours {
		result = append(result, w.Start.Add(time.Duration(offset)*time.Hour))
	}

	return result
}

const (
	// ForecastHorizonDays is the planning horizon we keep loaded for every place.
	ForecastHorizonDays = 10
	// ObservedHoursBack keeps the last few hours so "current" conditions survive
	// a gap between refreshes and short look-backs stay explainable.
	ObservedHoursBack = 6
)

// ForecastRange is a rolling horizon that always starts a few hours in the past
// and reaches ForecastHorizonDays ahead in Asia/Vladivostok. Unlike DayWindow it
// is not tied to one calendar day, so a refresh keeps both current observations
// and the whole forecast.
type ForecastRange struct {
	From     time.Time
	To       time.Time
	Today    DayWindow
	Location *time.Location
}

// NewForecastRange builds the rolling range around now.
func NewForecastRange(now time.Time) (ForecastRange, error) {
	loc, err := PrimoryeLocation()
	if err != nil {
		return ForecastRange{}, err
	}

	local := now.In(loc)
	today, err := NewDayWindow(local.Year(), local.Month(), local.Day())
	if err != nil {
		return ForecastRange{}, err
	}

	from := now.UTC().Truncate(time.Hour).Add(-ObservedHoursBack * time.Hour)
	lastDay := time.Date(local.Year(), local.Month(), local.Day()+ForecastHorizonDays-1, 0, 0, 0, 0, loc)
	to := time.Date(lastDay.Year(), lastDay.Month(), lastDay.Day()+1, 0, 0, 0, 0, loc).UTC()

	return ForecastRange{From: from, To: to, Today: today, Location: loc}, nil
}

// Days returns the calendar days covered by the range, today first.
func (r ForecastRange) Days() []DayWindow {
	days := make([]DayWindow, 0, ForecastHorizonDays)
	for offset := range ForecastHorizonDays {
		local := r.Today.Day.AddDate(0, 0, offset)
		day, err := NewDayWindow(local.Year(), local.Month(), local.Day())
		if err != nil {
			continue
		}
		days = append(days, day)
	}

	return days
}

// Contains reports whether the instant falls into the rolling range.
func (r ForecastRange) Contains(instant time.Time) bool {
	return !instant.Before(r.From) && instant.Before(r.To)
}

// Hours returns the number of hourly slots in the range.
func (r ForecastRange) Hours() int {
	return int(r.To.Sub(r.From) / time.Hour)
}
