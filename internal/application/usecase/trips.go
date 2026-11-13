package usecase

import (
	"context"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// ListTripsUseCase returns the trips of one user ordered by planned time.
type ListTripsUseCase struct {
	trips tripLister
}

type tripLister interface {
	ListByUser(ctx context.Context, filter models.TripFilter) ([]models.Trip, error)
}

func NewListTripsUseCase(trips tripLister) *ListTripsUseCase {
	return &ListTripsUseCase{trips: trips}
}

func (u *ListTripsUseCase) Do(ctx context.Context, filter models.TripFilter) ([]models.Trip, error) {
	validID, err := models.ValidateUUID("user_id", filter.UserID)
	if err != nil {
		return nil, err
	}

	filter.UserID = validID

	return u.trips.ListByUser(ctx, filter)
}

// CreateTripUseCase plans a new outing at a place.
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

	placeID, err := models.ValidateUUID("place_id", in.PlaceID)
	if err != nil {
		return models.Trip{}, err
	}

	if in.PlannedAt.IsZero() {
		return models.Trip{}, models.Invalid("`planned_at` is required")
	}

	if !in.Activity.Valid() {
		return models.Trip{}, models.Invalid("unexpected activity `" + string(in.Activity) + "`")
	}

	in.UserID = userID
	in.PlaceID = placeID
	in.PlannedAt = in.PlannedAt.UTC()

	return u.trips.Create(ctx, in)
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
