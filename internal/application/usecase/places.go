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
	if id == "" {
		return models.Place{}, models.Invalid("`id` is required")
	}

	// id may be a UUID or the human-readable slug from the URL.
	return u.places.GetByID(ctx, id)
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
	if err := validatePlaceScenario(in); err != nil {
		return models.Place{}, err
	}

	return u.places.Create(ctx, in)
}

// validatePlaceScenario keeps the catalog coherent: a category must belong
// to the place's scenario layer.
func validatePlaceScenario(in models.CreatePlaceInput) error {
	if in.Category == nil {
		return nil
	}
	if in.Category.Scenario() != in.Layer {
		return models.Invalidf(
			"category `%s` does not belong to layer `%s`",
			string(*in.Category),
			string(in.Layer),
		)
	}

	return nil
}

// ListSimilarPlacesUseCase returns the catalog entries most like one place.
type ListSimilarPlacesUseCase struct {
	places similarLister
}

type similarLister interface {
	ListSimilar(ctx context.Context, id string, limit int) ([]models.Place, error)
}

func NewListSimilarPlacesUseCase(places similarLister) *ListSimilarPlacesUseCase {
	return &ListSimilarPlacesUseCase{places: places}
}

func (u *ListSimilarPlacesUseCase) Do(ctx context.Context, id string, limit int) ([]models.Place, error) {
	if id == "" {
		return nil, models.Invalid("`id` is required")
	}
	if limit <= 0 || limit > maxSimilarPlaces {
		limit = defaultSimilarPlaces
	}

	return u.places.ListSimilar(ctx, id, limit)
}

const (
	defaultSimilarPlaces = 6
	maxSimilarPlaces     = 24
)

// PlaceStatsUseCase aggregates catalog counters for the directory tabs.
type PlaceStatsUseCase struct {
	places placeStatsReader
}

type placeStatsReader interface {
	Stats(ctx context.Context, layer *models.LayerCode) (models.PlaceStats, error)
}

func NewPlaceStatsUseCase(places placeStatsReader) *PlaceStatsUseCase {
	return &PlaceStatsUseCase{places: places}
}

func (u *PlaceStatsUseCase) Do(ctx context.Context, layer *models.LayerCode) (models.PlaceStats, error) {
	return u.places.Stats(ctx, layer)
}

// defaultPlaceListLimit mirrors the repository default so a missing `limit`
// parameter cannot request the whole table.
const defaultPlaceListLimit = 100
