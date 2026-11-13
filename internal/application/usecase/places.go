package usecase

import (
	"context"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// ListPlacesUseCase returns places filtered by layer, type or radius around a
// point. An empty filter lists the catalogue page by page.
type ListPlacesUseCase struct {
	places placeLister
}

type placeLister interface {
	List(ctx context.Context, filter models.PlaceFilter) ([]models.Place, error)
}

func NewListPlacesUseCase(places placeLister) *ListPlacesUseCase {
	return &ListPlacesUseCase{places: places}
}

func (u *ListPlacesUseCase) Do(ctx context.Context, filter models.PlaceFilter) ([]models.Place, error) {
	if filter.Limit <= 0 {
		filter.Limit = defaultPlaceListLimit
	}

	return u.places.List(ctx, filter)
}

// GetPlaceUseCase loads a single place by id.
type GetPlaceUseCase struct {
	places placeGetter
}

type placeGetter interface {
	GetByID(ctx context.Context, id string) (models.Place, error)
}

func NewGetPlaceUseCase(places placeGetter) *GetPlaceUseCase {
	return &GetPlaceUseCase{places: places}
}

func (u *GetPlaceUseCase) Do(ctx context.Context, id string) (models.Place, error) {
	validID, err := models.ValidateUUID("id", id)
	if err != nil {
		return models.Place{}, err
	}

	return u.places.GetByID(ctx, validID)
}

// CreatePlaceUseCase registers a new place. Input is already parsed by the
// HTTP layer into domain types.
type CreatePlaceUseCase struct {
	places placeCreator
}

type placeCreator interface {
	Create(ctx context.Context, in models.CreatePlaceInput) (models.Place, error)
}

func NewCreatePlaceUseCase(places placeCreator) *CreatePlaceUseCase {
	return &CreatePlaceUseCase{places: places}
}

func (u *CreatePlaceUseCase) Do(ctx context.Context, in models.CreatePlaceInput) (models.Place, error) {
	return u.places.Create(ctx, in)
}

// defaultPlaceListLimit mirrors the repository default so a missing `limit`
// parameter cannot request the whole table.
const defaultPlaceListLimit = 100
