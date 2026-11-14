package models

import "time"

// TripStatus is the lifecycle of a planned outing.
type TripStatus string

const (
	TripStatusPlanned   TripStatus = "planned"
	TripStatusDone      TripStatus = "done"
	TripStatusCancelled TripStatus = "cancelled"
)

// MaxTripStops mirrors the client route editor limit.
const MaxTripStops = 6

func NewTripStatus(raw string) (TripStatus, error) {
	status := TripStatus(raw)
	if !status.Valid() {
		return "", Invalid("unexpected trip status `" + raw + "`, expected: planned, done, cancelled")
	}

	return status, nil
}

func (s TripStatus) Valid() bool {
	switch s {
	case TripStatusPlanned, TripStatusDone, TripStatusCancelled:
		return true
	default:
		return false
	}
}

// TripStop is one ordered node of a planned route: the place and the
// estimated arrival time. Travel legs are computed by the client router, so
// the backend treats arrive_at as given.
type TripStop struct {
	PlaceID  string
	Position int
	ArriveAt time.Time
}

// Trip is a personal route plan: an ordered set of places visited one after
// another, starting at PlannedAt. A trip can be city, nature, sea or mixed —
// the scenario is defined by its stops.
type Trip struct {
	ID          string
	UserID      string
	PlannedAt   time.Time
	StopMinutes int
	Activity    Activity
	Status      TripStatus
	Notes       string
	Stops       []TripStop
	CreatedAt   time.Time
}

// CreateTripInput is the parsed payload of POST /trips.
type CreateTripInput struct {
	UserID      string
	PlannedAt   time.Time
	StopMinutes int
	Activity    Activity
	Notes       string
	Stops       []TripStop
}

// UpdateTripInput carries the optional status change of PATCH /trips/{id}.
// UserID is the owner from the session context: the update matches on both
// id and owner, so one user can never touch another user's trip.
type UpdateTripInput struct {
	ID     string
	UserID string
	Status *TripStatus
	Notes  *string
}

// TripFilter narrows the list of trips for a user.
type TripFilter struct {
	UserID string
	Limit  int
}

// TripStopConditions is one route node enriched with weather scoring: the
// cached assessment closest to the planned arrival and the best hour of that
// day, so the UI can highlight weak nodes and suggest a better slot.
type TripStopConditions struct {
	TripStop
	PlaceName   string
	PlaceSlug   string
	District    string
	Score       *Score
	Verdict     *Verdict
	Summary     string
	ScoreAt     *time.Time
	Warnings    []Warning
	BestScore   *Score
	BestVerdict *Verdict
	BestAt      *time.Time
}

// TripWithConditions is a planned route with every node scored — the "Мои
// походы" payload.
type TripWithConditions struct {
	Trip
	Stops []TripStopConditions
}
