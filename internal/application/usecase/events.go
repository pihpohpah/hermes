package usecase

import (
	"context"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// ListEventsUseCase returns catalog events, upcoming by default.
type ListEventsUseCase struct {
	events eventLister
	now    func() time.Time
}

type eventLister interface {
	List(ctx context.Context, filter models.EventFilter) ([]models.Event, error)
}

func NewListEventsUseCase(events eventLister, now func() time.Time) *ListEventsUseCase {
	return &ListEventsUseCase{events: events, now: now}
}

func (u *ListEventsUseCase) Do(ctx context.Context, filter models.EventFilter) ([]models.Event, error) {
	filter.Now = u.now()

	return u.events.List(ctx, filter)
}

// GetEventUseCase loads one event by UUID or slug.
type GetEventUseCase struct {
	events eventGetter
}

type eventGetter interface {
	GetByID(ctx context.Context, id string) (models.Event, error)
}

func NewGetEventUseCase(events eventGetter) *GetEventUseCase {
	return &GetEventUseCase{events: events}
}

func (u *GetEventUseCase) Do(ctx context.Context, id string) (models.Event, error) {
	if id == "" {
		return models.Event{}, models.Invalid("`id` is required")
	}

	return u.events.GetByID(ctx, id)
}

// ListCollectionsUseCase returns editorial collections, optionally for one
// scenario (collections without a scenario apply to all).
type ListCollectionsUseCase struct {
	collections collectionLister
}

type collectionLister interface {
	List(ctx context.Context, scenario *models.LayerCode) ([]models.Collection, error)
}

func NewListCollectionsUseCase(collections collectionLister) *ListCollectionsUseCase {
	return &ListCollectionsUseCase{collections: collections}
}

func (u *ListCollectionsUseCase) Do(ctx context.Context, scenario *models.LayerCode) ([]models.Collection, error) {
	return u.collections.List(ctx, scenario)
}

// GetCollectionUseCase loads one collection with its ordered item references.
type GetCollectionUseCase struct {
	collections collectionGetter
}

type collectionGetter interface {
	GetWithItems(ctx context.Context, id string) (models.Collection, error)
}

func NewGetCollectionUseCase(collections collectionGetter) *GetCollectionUseCase {
	return &GetCollectionUseCase{collections: collections}
}

func (u *GetCollectionUseCase) Do(ctx context.Context, id string) (models.Collection, error) {
	if id == "" {
		return models.Collection{}, models.Invalid("`id` is required")
	}

	return u.collections.GetWithItems(ctx, id)
}
