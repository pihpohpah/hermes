package models

import (
	"fmt"
	"strings"
	"time"
)

// PinKind is the kind of an ephemeral "now" radar pin. It is a separate
// vocabulary from Activity (hike/sup/run/walk): kinds describe what people
// are gathering for, not what the weather is scored for.
type PinKind string

const (
	PinKindSport PinKind = "sport"
	PinKindWalk  PinKind = "walk"
	PinKindMusic PinKind = "music"
	PinKindGames PinKind = "games"
	PinKindHelp  PinKind = "help"
	PinKindOther PinKind = "other"
)

func NewPinKind(raw string) (PinKind, error) {
	kind := PinKind(strings.TrimSpace(raw))
	if !kind.Valid() {
		return "", Invalid(fmt.Sprintf(
			"unexpected pin kind `%s`, expected: sport, walk, music, games, help, other", raw,
		))
	}

	return kind, nil
}

func (k PinKind) Valid() bool {
	switch k {
	case PinKindSport, PinKindWalk, PinKindMusic, PinKindGames, PinKindHelp, PinKindOther:
		return true
	default:
		return false
	}
}

const (
	maxPinTitleLength  = 140
	maxPinNoteLength   = 140
	maxPinPlaceLabel   = 140
	maxPinReasonLength = 280
	maxPinNeed         = 100
)

// PinBounds is the geographic viewport of a radar request: WGS84 degrees.
type PinBounds struct {
	MinLon float64
	MinLat float64
	MaxLon float64
	MaxLat float64
}

func NewPinBounds(minLon, minLat, maxLon, maxLat float64) (PinBounds, error) {
	if minLon >= maxLon || minLat >= maxLat {
		return PinBounds{}, Invalid("`bbox` must be minLon,minLat,maxLon,maxLat with min < max")
	}

	bounds := PinBounds{MinLon: minLon, MinLat: minLat, MaxLon: maxLon, MaxLat: maxLat}
	for _, point := range []Point{
		{Lat: minLat, Lon: minLon},
		{Lat: maxLat, Lon: maxLon},
	} {
		if _, err := NewPoint(point.Lat, point.Lon); err != nil {
			return PinBounds{}, err
		}
	}

	return bounds, nil
}

// Pin is a temporary invitation marker on the radar map. It disappears after
// EndsAt. Going counts participants other than the author; the author always
// attends, so GoingTotal is Going + 1.
type Pin struct {
	ID         string
	Kind       PinKind
	Title      string
	Note       string
	PlaceLabel string
	Location   Point
	StartsAt   time.Time
	EndsAt     time.Time
	Need       *int
	Going      int
	AuthorID   string
	AuthorName string
	CreatedAt  time.Time

	// Viewer decoration; filled for the requesting user when known.
	Joined     bool
	Mine       bool
	GoingTotal int
}

// CreatePinInput is the parsed payload of POST /pins plus the author from
// the session.
type CreatePinInput struct {
	Kind       PinKind
	Title      string
	Note       string
	PlaceLabel string
	Location   Point
	StartsAt   time.Time
	EndsAt     time.Time
	Need       *int
	AuthorID   string
}

// Validate checks shape only; moderation rules (limits, stop words, night
// hours) live in the use case where configuration is available.
func (in CreatePinInput) Validate(maxDuration time.Duration) error {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Invalid("`title` is required")
	}
	if len(title) > maxPinTitleLength {
		return Invalidf("`title` must be at most %d characters", maxPinTitleLength)
	}
	if len(in.Note) > maxPinNoteLength {
		return Invalidf("`note` must be at most %d characters", maxPinNoteLength)
	}
	if len(in.PlaceLabel) > maxPinPlaceLabel {
		return Invalidf("`place_label` must be at most %d characters", maxPinPlaceLabel)
	}
	if !in.Kind.Valid() {
		return Invalid("unexpected pin kind `" + string(in.Kind) + "`")
	}
	if in.StartsAt.IsZero() || in.EndsAt.IsZero() {
		return Invalid("`starts_at` and `ends_at` are required")
	}
	if !in.EndsAt.After(in.StartsAt) {
		return Invalid("`ends_at` must be after `starts_at`")
	}
	if maxDuration > 0 && in.EndsAt.Sub(in.StartsAt) > maxDuration {
		return Invalidf("pin duration must be at most %s", maxDuration)
	}
	if in.Need != nil && (*in.Need < 0 || *in.Need > maxPinNeed) {
		return Invalidf("`need` must be between 0 and %d", maxPinNeed)
	}

	return nil
}

// PinFilter narrows the radar list. Viewer decorates the pins for that user.
type PinFilter struct {
	Bounds *PinBounds
	Kind   *PinKind
	Viewer *string
	Limit  int
	Now    time.Time
}

// SetGoingInput is one user's answer to a pin.
type SetGoingInput struct {
	PinID  string
	UserID string
	Going  bool
	Now    time.Time
}

// CreatePinReportInput is a complaint about a pin. One report per user per pin.
type CreatePinReportInput struct {
	PinID  string
	UserID string
	Reason string
	Now    time.Time
}

func (in CreatePinReportInput) Validate() error {
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return Invalid("`reason` is required")
	}
	if len(reason) > maxPinReasonLength {
		return Invalidf("`reason` must be at most %d characters", maxPinReasonLength)
	}

	return nil
}

// PinReport is one complaint row.
type PinReport struct {
	ID        string
	PinID     string
	UserID    string
	Reason    string
	CreatedAt time.Time
}
