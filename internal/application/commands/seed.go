package commands

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pihpohpah/hermes/internal/application/commands/seeddata"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

type seedPlaceRepo interface {
	Create(ctx context.Context, in models.CreatePlaceInput) (models.Place, error)
}

// seedEventRepo сидит редакционные события макета. Upsert по slug, чтобы
// повторный seed обновлял те же строки.
type seedEventRepo interface {
	UpsertBySlug(ctx context.Context, event models.Event) (models.Event, error)
}

// SeedCommand наполняет каталог контентом дизайн-прототипа (vlru-places):
// ручные outdoor-сиды, все места, прогулки и события макета. Weather and
// scoring are separate commands, so seeding never produces fake "ready"
// recommendations.
type SeedCommand struct {
	logger *slog.Logger
	places seedPlaceRepo
	events seedEventRepo
}

func NewSeedCommand(logger *slog.Logger, places seedPlaceRepo, events seedEventRepo) *SeedCommand {
	return &SeedCommand{logger: logger, places: places, events: events}
}

func (c *SeedCommand) Run(ctx context.Context) error {
	// placeIDsBySlug собирает slug → id всех засиженных мест: события макета
	// ссылаются на площадки именно слагами из places.json.
	placeIDsBySlug, err := c.seedPlaces(ctx)
	if err != nil {
		return err
	}

	return c.seedEvents(ctx, placeIDsBySlug)
}

func (c *SeedCommand) seedPlaces(ctx context.Context) (map[string]string, error) {
	mockupPlaces, err := seeddata.MockupPlaces()
	if err != nil {
		return nil, err
	}

	ids := make(map[string]string, len(mockupPlaces)+8)
	seed := func(in models.CreatePlaceInput) error {
		place, err := c.places.Create(ctx, in)
		if err != nil {
			return fmt.Errorf("seed place `%s`: %w", in.Name, err)
		}
		if place.Slug != nil {
			ids[*place.Slug] = place.ID
		}

		c.logger.Info(
			"seeded place",
			slog.String("id", place.ID),
			slog.String("name", place.Name),
			slog.String("type", string(place.Type)),
		)

		return nil
	}

	for _, item := range vladivostokSeeds() {
		if err := seed(item); err != nil {
			return nil, err
		}
	}
	for _, item := range mockupPlaces {
		if err := seed(item); err != nil {
			return nil, err
		}
	}

	return ids, nil
}

func (c *SeedCommand) seedEvents(ctx context.Context, placeIDsBySlug map[string]string) error {
	events, placeSlugs, err := seeddata.MockupEvents()
	if err != nil {
		return err
	}

	for i, event := range events {
		if slug := placeSlugs[i]; slug != "" {
			if placeID, ok := placeIDsBySlug[slug]; ok {
				event.PlaceID = stringPtr(placeID)
			} else {
				c.logger.Warn("mockup event venue slug not found among seeded places", slog.String("event", event.Title), slog.String("slug", slug))
			}
		}

		saved, err := c.events.UpsertBySlug(ctx, event)
		if err != nil {
			return fmt.Errorf("seed event `%s`: %w", event.Title, err)
		}

		c.logger.Info(
			"seeded event",
			slog.String("id", saved.ID),
			slog.String("title", saved.Title),
			slog.String("category", string(saved.Category)),
		)
	}

	return nil
}

func stringPtr(value string) *string { return &value }

const (
	tokarevskyLat  = 43.0732
	tokarevskyLon  = 131.8431
	tokarevskyElev = 2
	tokarevskyDiff = models.Difficulty(2)
	tokarevskyOSM  = int64(1000001)

	tobizinaLat  = 42.9825
	tobizinaLon  = 131.8794
	tobizinaElev = 42
	tobizinaDiff = models.Difficulty(3)
	tobizinaOSM  = int64(1000002)

	nagornyLat  = 43.1186
	nagornyLon  = 131.9082
	nagornyElev = 145
	nagornyDiff = models.Difficulty(1)
	nagornyOSM  = int64(1000003)

	burachkaLat  = 43.1003
	burachkaLon  = 131.9061
	burachkaElev = 138
	burachkaDiff = models.Difficulty(1)
	burachkaOSM  = int64(1000004)

	steklyannayaLat  = 43.1636
	steklyannayaLon  = 132.0497
	steklyannayaElev = 3
	steklyannayaDiff = models.Difficulty(1)
	steklyannayaOSM  = int64(1000005)
)

func vladivostokSeeds() []models.CreatePlaceInput {
	intPtr := func(v int) *int { return &v }
	int64Ptr := func(v int64) *int64 { return &v }
	diffPtr := func(v models.Difficulty) *models.Difficulty { return &v }

	return []models.CreatePlaceInput{
		{
			Name:        "Маяк Токаревский (Сап-спот)",
			Description: "Точка старта на косе Токаревского маяка. Утром гладкая вода и виды на Босфор Восточный.",
			Type:        models.PlaceTypeSupSpot,
			Layer:       models.LayerSea,
			Location:    models.Point{Lat: tokarevskyLat, Lon: tokarevskyLon},
			Elevation:   intPtr(tokarevskyElev),
			Difficulty:  diffPtr(tokarevskyDiff),
			OSMID:       int64Ptr(tokarevskyOSM),
		},
		{
			Name:        "Мыс Тобизина",
			Description: "Тропа вдоль скалистых обрывов острова Русский к открытому морю и маяку.",
			Type:        models.PlaceTypeTrailhead,
			Layer:       models.LayerNature,
			Location:    models.Point{Lat: tobizinaLat, Lon: tobizinaLon},
			Elevation:   intPtr(tobizinaElev),
			Difficulty:  diffPtr(tobizinaDiff),
			OSMID:       int64Ptr(tobizinaOSM),
		},
		{
			Name:        "Нагорный парк",
			Description: "Благоустроенный городской парк на сопке с панорамной видовой площадкой на бухту Золотой Рог.",
			Type:        models.PlaceTypePark,
			Layer:       models.LayerCity,
			Location:    models.Point{Lat: nagornyLat, Lon: nagornyLon},
			Elevation:   intPtr(nagornyElev),
			Difficulty:  diffPtr(nagornyDiff),
			OSMID:       int64Ptr(nagornyOSM),
		},
		{
			Name:        "Сопка Бурачка",
			Description: "Оборудованная смотровая площадка с металлической лестницей и круговым обзором 360 градусов.",
			Type:        models.PlaceTypeViewpoint,
			Layer:       models.LayerCity,
			Location:    models.Point{Lat: burachkaLat, Lon: burachkaLon},
			Elevation:   intPtr(burachkaElev),
			Difficulty:  diffPtr(burachkaDiff),
			OSMID:       int64Ptr(burachkaOSM),
		},
		{
			Name:        "Бухта Стеклянная",
			Description: "Пляж с обкатанным морским стеклом и удобным выходом к воде в Уссурийском заливе.",
			Type:        models.PlaceTypeBeach,
			Layer:       models.LayerSea,
			Location:    models.Point{Lat: steklyannayaLat, Lon: steklyannayaLon},
			Elevation:   intPtr(steklyannayaElev),
			Difficulty:  diffPtr(steklyannayaDiff),
			OSMID:       int64Ptr(steklyannayaOSM),
		},
	}
}
