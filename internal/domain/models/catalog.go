package models

import (
	"fmt"
	"strings"
	"time"
)

// CategoryCode is a catalog category of a place within its scenario
// (city/nature/sea), mirroring the taxonomy of the design prototype.
type CategoryCode string

func NewCategoryCode(raw string) (CategoryCode, error) {
	code := CategoryCode(strings.TrimSpace(raw))
	if _, ok := placeCategoryTitles[code]; !ok {
		return "", Invalid(fmt.Sprintf("unexpected category `%s`", raw))
	}

	return code, nil
}

// Scenario returns the scenario a category belongs to.
func (c CategoryCode) Scenario() LayerCode {
	return placeCategoryScenarios[c]
}

// placeCategoryTitles lists every valid category per scenario. It is the
// domain copy of taxonomy.js in the design prototype; the same rows are
// seeded into place_categories by the migration.
var placeCategoryTitles = map[CategoryCode]string{
	"walks":       "Прогулки",
	"viewpoints":  "Смотровые",
	"history":     "История",
	"museums":     "Музеи",
	"art":         "Арт-пространства",
	"stage":       "Музыка и сцена",
	"hiking":      "Сопки и тропы",
	"waterfalls":  "Водопады",
	"lakes":       "Озёра",
	"parks":       "Парки и сады",
	"capes":       "Мысы и скалы",
	"reserves":    "Заповедные места",
	"beaches":     "Пляжи",
	"bays":        "Бухты",
	"lighthouses": "Маяки",
	"islands":     "Острова",
	"diving":      "Дайвинг",
	"boats":       "Морские прогулки",
}

var placeCategoryScenarios = map[CategoryCode]LayerCode{
	"walks":       LayerCity,
	"viewpoints":  LayerCity,
	"history":     LayerCity,
	"museums":     LayerCity,
	"art":         LayerCity,
	"stage":       LayerCity,
	"hiking":      LayerNature,
	"waterfalls":  LayerNature,
	"lakes":       LayerNature,
	"parks":       LayerNature,
	"capes":       LayerNature,
	"reserves":    LayerNature,
	"beaches":     LayerSea,
	"bays":        LayerSea,
	"lighthouses": LayerSea,
	"islands":     LayerSea,
	"diving":      LayerSea,
	"boats":       LayerSea,
}

// CategoriesOf returns the valid categories for one scenario.
func CategoriesOf(layer LayerCode) []CategoryCode {
	out := make([]CategoryCode, 0, len(placeCategoryTitles))
	for code, scenario := range placeCategoryScenarios {
		if scenario == layer {
			out = append(out, code)
		}
	}

	return out
}

// EventCategoryCode is a category of an event.
type EventCategoryCode string

func NewEventCategoryCode(raw string) (EventCategoryCode, error) {
	code := EventCategoryCode(strings.TrimSpace(raw))
	if _, ok := eventCategoryTitles[code]; !ok {
		return "", Invalid(fmt.Sprintf("unexpected event category `%s`", raw))
	}

	return code, nil
}

var eventCategoryTitles = map[EventCategoryCode]string{
	"concert":    "Концерт",
	"exhibition": "Выставка",
	"tour":       "Экскурсия",
	"festival":   "Фестиваль",
	"sport":      "Спорт",
	"party":      "Андеграунд",
	"lecture":    "Лекция",
	"market":     "Маркет",
	// Категории импорта event-agent (webscraper).
	"theater":    "Театр",
	"workshop":   "Мастер-класс",
	"meetup":     "Митап",
	"conference": "Конференция",
	"standup":    "Стендап",
	"quiz":       "Квиз",
	"kids":       "Детям",
	"film":       "Кино",
	"other":      "Разное",
}

// EventStatus is the publication state of an event. The CHECK constraint in
// the migration mirrors the values; extend both sides together.
type EventStatus string

const (
	EventStatusScheduled EventStatus = "scheduled"
	EventStatusCancelled EventStatus = "cancelled"
)

func NewEventStatus(raw string) (EventStatus, error) {
	status := EventStatus(strings.TrimSpace(raw))
	switch status {
	case EventStatusScheduled, EventStatusCancelled:
		return status, nil
	default:
		return "", Invalid(fmt.Sprintf("unexpected event status `%s`", raw))
	}
}

// AccessMode is a way to reach a place.
type AccessMode string

const (
	AccessWalk    AccessMode = "walk"
	AccessTransit AccessMode = "transit"
	AccessCar     AccessMode = "car"
	AccessFerry   AccessMode = "ferry"
	AccessBoat    AccessMode = "boat"
)

func NewAccessMode(raw string) (AccessMode, error) {
	mode := AccessMode(strings.TrimSpace(raw))
	switch mode {
	case AccessWalk, AccessTransit, AccessCar, AccessFerry, AccessBoat:
		return mode, nil
	default:
		return "", Invalid(fmt.Sprintf(
			"unexpected access mode `%s`, expected: walk, transit, car, ferry, boat", raw,
		))
	}
}

// BeachType is the shore type of a sea place.
type BeachType string

const (
	BeachSand   BeachType = "sand"
	BeachPebble BeachType = "pebble"
	BeachRocks  BeachType = "rocks"
	BeachNone   BeachType = "none"
)

func NewBeachType(raw string) (BeachType, error) {
	beach := BeachType(strings.TrimSpace(raw))
	switch beach {
	case BeachSand, BeachPebble, BeachRocks, BeachNone:
		return beach, nil
	default:
		return "", Invalid(fmt.Sprintf(
			"unexpected beach type `%s`, expected: sand, pebble, rocks, none", raw,
		))
	}
}

// Photo is photo metadata attached to a place or event. Files themselves are
// stored elsewhere; this is the catalog card reference with attribution.
type Photo struct {
	Src        string `json:"src"`
	SrcSm      string `json:"src_sm,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	Alt        string `json:"alt,omitempty"`
	Author     string `json:"author,omitempty"`
	License    string `json:"license,omitempty"`
	LicenseURL string `json:"license_url,omitempty"`
	Source     string `json:"source,omitempty"`
}

// SrcSmOrSrc returns the small variant when present, falling back to the
// full source.
func (p Photo) SrcSmOrSrc() string {
	if p.SrcSm != "" {
		return p.SrcSm
	}

	return p.Src
}

// Event is a dated happening, optionally attached to a place.
type Event struct {
	ID          string
	Slug        *string
	Title       string
	Category    EventCategoryCode
	Short       string
	Description string
	PlaceID     *string
	PlaceName   *string
	Venue       string
	Organizer   string
	Age         string
	StartsAt    time.Time
	EndsAt      time.Time
	CreatedAt   time.Time
	// ExternalID is the stable key of the importing service (event-agent run
	// event id); it backs the idempotent upsert. Nil for editorial events.
	ExternalID *string
	// Location is the geocoded venue coordinate; nil when the venue text is
	// missing or geocoding found nothing.
	Location *Point
	// Status is the publication state (cancelled events stay listed, so the
	// catalog can say «отменено» instead of silently dropping the event).
	Status EventStatus
	// PriceFree mirrors places.price_free: nil means «unknown».
	PriceFree  *bool
	PriceLabel string
	// EventURL is the canonical page of the event, http(s) only.
	EventURL *string
	// SourceURLs maps a source domain to the page URL where the event was
	// found; filled by the event-agent import.
	SourceURLs map[string]string
}

// EventFilter narrows the event list. UpcomingOnly (default true) excludes
// events whose EndsAt has passed.
type EventFilter struct {
	PlaceID      *string
	Category     *EventCategoryCode
	Q            string
	UpcomingOnly bool
	Limit        int
	Now          time.Time
}

// CollectionItemRef points at one catalog entry inside a collection.
type CollectionItemRef struct {
	ItemType string // "place" or "event"
	ItemID   string
	Position int
}

// Collection is an editorial list of catalog entries.
type Collection struct {
	ID       string
	Slug     *string
	Title    string
	Scenario *LayerCode
	Cover    string
	Items    []CollectionItemRef
}

// PlaceStats is the catalog aggregate behind the directory tabs: entry
// counts grouped by category, plus the place/event split.
type PlaceStats struct {
	Places              int
	Events              int
	CategoryCounts      map[string]int
	EventCategoryCounts map[string]int
}
