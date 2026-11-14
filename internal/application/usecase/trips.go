package usecase

import (
	"context"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// ListTripsUseCase returns the route plans of one user ordered by planned
// time, every node scored against the cached conditions.
type ListTripsUseCase struct {
	trips tripConditionLister
}

type tripConditionLister interface {
	ListByUserWithConditions(ctx context.Context, filter models.TripFilter) ([]models.TripWithConditions, error)
}

func NewListTripsUseCase(trips tripConditionLister) *ListTripsUseCase {
	return &ListTripsUseCase{trips: trips}
}

func (u *ListTripsUseCase) Do(ctx context.Context, filter models.TripFilter) ([]models.TripWithConditions, error) {
	validID, err := models.ValidateUUID("user_id", filter.UserID)
	if err != nil {
		return nil, err
	}

	filter.UserID = validID

	return u.trips.ListByUserWithConditions(ctx, filter)
}

// CreateTripUseCase plans a new route trip: an ordered set of places with
// estimated arrival times (travel legs are computed client-side).
type CreateTripUseCase struct {
	trips tripCreator
}

type tripCreator interface {
	Create(ctx context.Context, in models.CreateTripInput) (models.Trip, error)
}

func NewCreateTripUseCase(trips tripCreator) *CreateTripUseCase {
	return &CreateTripUseCase{trips: trips}
}

func (u *CreateTripUseCase) Do(ctx context.Context, in models.CreateTripInput) (models.Trip, error) {
	userID, err := models.ValidateUUID("user_id", in.UserID)
	if err != nil {
		return models.Trip{}, err
	}

	if in.PlannedAt.IsZero() {
		return models.Trip{}, models.Invalid("`planned_at` is required")
	}

	if !in.Activity.Valid() {
		return models.Trip{}, models.Invalid("unexpected activity `" + string(in.Activity) + "`")
	}

	if len(in.Stops) == 0 || len(in.Stops) > models.MaxTripStops {
		return models.Trip{}, models.Invalid(
			"`stops` must contain between 1 and 6 places",
		)
	}

	seen := make(map[string]struct{}, len(in.Stops))
	for i := range in.Stops {
		stop := &in.Stops[i]

		placeID, err := models.ValidateUUID("place_id", stop.PlaceID)
		if err != nil {
			return models.Trip{}, err
		}
		if _, dup := seen[placeID]; dup {
			return models.Trip{}, models.Invalid("duplicate stop `" + placeID + "`")
		}
		seen[placeID] = struct{}{}

		if stop.ArriveAt.IsZero() {
			return models.Trip{}, models.Invalid("`arrive_at` is required for every stop")
		}

		stop.PlaceID = placeID
		stop.Position = i
		stop.ArriveAt = stop.ArriveAt.UTC()
	}

	if in.StopMinutes <= 0 {
		in.StopMinutes = 60
	}

	in.UserID = userID
	in.PlannedAt = in.PlannedAt.UTC()

	return u.trips.Create(ctx, in)
}

// GetTripUseCase loads one route plan with scored stops, owner-scoped.
type GetTripUseCase struct {
	trips tripConditionGetter
}

type tripConditionGetter interface {
	GetByIDWithConditions(ctx context.Context, id string, userID string) (models.TripWithConditions, error)
}

func NewGetTripUseCase(trips tripConditionGetter) *GetTripUseCase {
	return &GetTripUseCase{trips: trips}
}

func (u *GetTripUseCase) Do(ctx context.Context, id string, userID string) (models.TripWithConditions, error) {
	validID, err := models.ValidateUUID("id", id)
	if err != nil {
		return models.TripWithConditions{}, err
	}

	validUserID, err := models.ValidateUUID("user_id", userID)
	if err != nil {
		return models.TripWithConditions{}, err
	}

	return u.trips.GetByIDWithConditions(ctx, validID, validUserID)
}

// UpdateTripUseCase changes the status or notes of an existing trip.
type UpdateTripUseCase struct {
	trips tripUpdater
}

type tripUpdater interface {
	Update(ctx context.Context, in models.UpdateTripInput) (models.Trip, error)
}

func NewUpdateTripUseCase(trips tripUpdater) *UpdateTripUseCase {
	return &UpdateTripUseCase{trips: trips}
}

func (u *UpdateTripUseCase) Do(ctx context.Context, in models.UpdateTripInput) (models.Trip, error) {
	validID, err := models.ValidateUUID("id", in.ID)
	if err != nil {
		return models.Trip{}, err
	}

	userID, err := models.ValidateUUID("user_id", in.UserID)
	if err != nil {
		return models.Trip{}, err
	}

	in.ID = validID
	in.UserID = userID

	return u.trips.Update(ctx, in)
}
