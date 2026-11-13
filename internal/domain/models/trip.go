package models

import "time"

// TripStatus is the lifecycle of a planned outing.
type TripStatus string

const (
	TripStatusPlanned   TripStatus = "planned"
	TripStatusDone      TripStatus = "done"
	TripStatusCancelled TripStatus = "cancelled"
)

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

// Trip is a personal plan to visit a place at a given time.
type Trip struct {
	ID        string
	UserID    string
	PlaceID   string
	PlannedAt time.Time
	Activity  Activity
	Status    TripStatus
	Notes     string
	CreatedAt time.Time
}

// CreateTripInput is the parsed payload of POST /trips.
type CreateTripInput struct {
	UserID    string
	PlaceID   string
	PlannedAt time.Time
	Activity  Activity
	Notes     string
}

// UpdateTripInput carries the optional status change of PATCH /trips/{id}.
type UpdateTripInput struct {
	ID     string
	Status *TripStatus
	Notes  *string
}

// TripFilter narrows the list of trips for a user.
type TripFilter struct {
	UserID string
	Limit  int
}
