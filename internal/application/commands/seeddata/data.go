// Package seeddata — контент каталога, перенесённый из дизайн-прототипа
// vlru-places (data/places.json и data/events.json макета). JSON копируются
// в пакет как есть: go:embed не умеет выходить за пределы директории пакета,
// а держать парсер рядом с данными проще, чем тянуть зависимость на папку
// макета из production-бинаря. При правке контента в макете перекопируйте
// файлы сюда.
package seeddata

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

//go:embed places.json
var placesJSON []byte

//go:embed events.json
var eventsJSON []byte

type seedPrice struct {
	Free  bool   `json:"free"`
	Label string `json:"label"`
}

// seedPlace зеркалит запись data/places.json макета.
type seedPlace struct {
	ID          string      `json:"id"`
	Scenario    string      `json:"scenario"`
	Category    string      `json:"category"`
	Title       string      `json:"title"`
	Subtitle    string      `json:"subtitle"`
	Short       string      `json:"short"`
	Description []string    `json:"description"`
	District    string      `json:"district"`
	Address     string      `json:"address"`
	Hours       string      `json:"hours"`
	Price       *seedPrice  `json:"price"`
	Duration    string      `json:"duration"`
	Season      string      `json:"season"`
	Access      []string    `json:"access"`
	HowToGet    string      `json:"howToGet"`
	Facts       []string    `json:"facts"`
	Tags        []string    `json:"tags"`
	Beach       string      `json:"beach"`
	SwimSeason  string      `json:"swimSeason"`
	Difficulty  string      `json:"difficulty"`
	Coords      [2]float64  `json:"coords"` // [lon, lat], как в GeoJSON
}

// seedEvent зеркалит запись data/events.json макета.
type seedEvent struct {
	ID          string      `json:"id"`
	Category    string      `json:"category"`
	Title       string      `json:"title"`
	Short       string      `json:"short"`
	Description []string    `json:"description"`
	PlaceID     string      `json:"placeId"` // slug места из places.json, может быть пустым
	Venue       string      `json:"venue"`
	Organizer   string      `json:"organizer"`
	Age         string      `json:"age"`
	StartsAt    string      `json:"startsAt"`
	EndsAt      string      `json:"endsAt"`
	Price       *seedPrice  `json:"price"`
	Coords      [2]float64  `json:"coords"`
}

// skippedMockupPlaces — места макета, которые не сидят повторно: эти объекты
// уже покрыты ручными сидами в seed.go (с более полными outdoor-полями:
// высота, сложность, тип точки). Слаг «mys-tobizina» совпадает с slug ручного
// сида «Мыс Тобизина», поэтому привязка событий к этому месту сохраняется.
var skippedMockupPlaces = map[string]bool{
	"mys-tobizina":     true,
	"nagorny-park":     true,
	"tokarevskiy-mayak": true,
}

// mockupBaseOSMID — синтетический «osm_id» первого места макета. Ручные сиды
// занимают 1000001–1000005; макетные идут от 2000000. Idempotent upsert
// PlaceRepository.Create конфликтует по osm_id, поэтому повторный seed
// обновляет те же строки, а не плодит дубли.
const mockupBaseOSMID = int64(2000000)

func parsePlaces() ([]seedPlace, error) {
	var places []seedPlace
	if err := json.Unmarshal(placesJSON, &places); err != nil {
		return nil, fmt.Errorf("parse seeddata/places.json: %w", err)
	}

	return places, nil
}

func parseEvents() ([]seedEvent, error) {
	var events []seedEvent
	if err := json.Unmarshal(eventsJSON, &events); err != nil {
		return nil, fmt.Errorf("parse seeddata/events.json: %w", err)
	}

	return events, nil
}

// placeTypeForCategory сужает категорию макета до типа точки домена (их
// всего пять — это ядро outdoor-сервиса, а не полный таксономия макета).
func placeTypeForCategory(category string) models.PlaceType {
	switch category {
	case "beaches", "bays":
		return models.PlaceTypeBeach
	case "viewpoints", "capes", "lighthouses":
		return models.PlaceTypeViewpoint
	case "hiking", "reserves", "waterfalls", "lakes", "islands":
		return models.PlaceTypeTrailhead
	case "diving", "boats":
		return models.PlaceTypeSupSpot
	default: // walks, history, museums, art, stage, parks
		return models.PlaceTypePark
	}
}

// difficultyForSeed переводит подпись сложности макета в шкалу домена 1–5;
// границы зеркалят presenter.difficultyCode (easy ≤ 2, medium = 3, hard ≥ 4).
func difficultyForSeed(raw string) *models.Difficulty {
	var value models.Difficulty
	switch raw {
	case "easy":
		value = 2
	case "medium":
		value = 3
	case "hard":
		value = 4
	default:
		return nil
	}

	return &value
}

func joinParagraphs(parts []string) string {
	return strings.Join(parts, "\n\n")
}

// MockupPlaces возвращает места макета в виде входов создания места. osmID
// присваивается последовательно от mockupBaseOSMID — вызывающий код передаёт
// его как есть, idempotent upsert сделает остальное.
func MockupPlaces() ([]models.CreatePlaceInput, error) {
	places, err := parsePlaces()
	if err != nil {
		return nil, err
	}

	out := make([]models.CreatePlaceInput, 0, len(places))
	osmID := mockupBaseOSMID
	for _, item := range places {
		if skippedMockupPlaces[item.ID] {
			continue
		}
		osmID++

		category, err := models.NewCategoryCode(item.Category)
		if err != nil {
			return nil, fmt.Errorf("mockup place `%s`: %w", item.ID, err)
		}
		layer, err := models.NewLayerCode(item.Scenario)
		if err != nil {
			return nil, fmt.Errorf("mockup place `%s`: %w", item.ID, err)
		}

		in := models.CreatePlaceInput{
			Name:        item.Title,
			Slug:        stringPtr(item.ID),
			Subtitle:    item.Subtitle,
			Short:       item.Short,
			Description: joinParagraphs(item.Description),
			Type:        placeTypeForCategory(item.Category),
			Layer:       layer,
			Category:    &category,
			District:    item.District,
			Address:     item.Address,
			Location:    models.Point{Lat: item.Coords[1], Lon: item.Coords[0]},
			Hours:       item.Hours,
			PriceLabel:  priceLabel(item.Price),
			PriceFree:   priceFree(item.Price),
			Duration:    item.Duration,
			Season:      item.Season,
			Access:      accessModes(item.Access),
			HowToGet:    item.HowToGet,
			Facts:       item.Facts,
			Tags:        item.Tags,
			SwimSeason:  item.SwimSeason,
			Difficulty:  difficultyForSeed(item.Difficulty),
			OSMID:       int64Ptr(osmID),
		}
		if item.Beach != "" {
			beach, err := models.NewBeachType(item.Beach)
			if err != nil {
				return nil, fmt.Errorf("mockup place `%s`: %w", item.ID, err)
			}
			in.Beach = &beach
		}

		out = append(out, in)
	}

	return out, nil
}

// MockupEvents возвращает события макета в виде доменных событий. placeSlug
// — slug места проведения из places.json (пустая строка, когда событие без
// привязки); вызывающий код должен сам разрешить его в id места в базе.
func MockupEvents() ([]models.Event, []string, error) {
	items, err := parseEvents()
	if err != nil {
		return nil, nil, err
	}

	events := make([]models.Event, 0, len(items))
	placeSlugs := make([]string, len(items))
	for i, item := range items {
		category, err := models.NewEventCategoryCode(item.Category)
		if err != nil {
			return nil, nil, fmt.Errorf("mockup event `%s`: %w", item.ID, err)
		}
		startsAt, err := time.Parse(time.RFC3339, item.StartsAt)
		if err != nil {
			return nil, nil, fmt.Errorf("mockup event `%s` starts_at: %w", item.ID, err)
		}
		endsAt, err := time.Parse(time.RFC3339, item.EndsAt)
		if err != nil {
			return nil, nil, fmt.Errorf("mockup event `%s` ends_at: %w", item.ID, err)
		}

		event := models.Event{
			Title:       item.Title,
			Slug:        stringPtr(item.ID),
			Category:    category,
			Short:       item.Short,
			Description: joinParagraphs(item.Description),
			Venue:       item.Venue,
			Organizer:   item.Organizer,
			Age:         item.Age,
			StartsAt:    startsAt,
			EndsAt:      endsAt,
			Status:      models.EventStatusScheduled,
			PriceFree:   priceFree(item.Price),
			PriceLabel:  priceLabel(item.Price),
		}
		if item.Coords != [2]float64{} {
			event.Location = &models.Point{Lat: item.Coords[1], Lon: item.Coords[0]}
		}

		events = append(events, event)
		placeSlugs[i] = item.PlaceID
	}

	return events, placeSlugs, nil
}

func accessModes(raw []string) []models.AccessMode {
	out := make([]models.AccessMode, 0, len(raw))
	for _, mode := range raw {
		parsed, err := models.NewAccessMode(mode)
		if err != nil {
			// Макет контролируется редакцией: неизвестный способ доступа
			// пропускаем, а не роняем весь seed.
			continue
		}
		out = append(out, parsed)
	}

	return out
}

func priceFree(price *seedPrice) *bool {
	if price == nil {
		return nil
	}

	return &price.Free
}

func priceLabel(price *seedPrice) string {
	if price == nil {
		return ""
	}

	return price.Label
}

func stringPtr(value string) *string { return &value }

func int64Ptr(value int64) *int64 { return &value }
