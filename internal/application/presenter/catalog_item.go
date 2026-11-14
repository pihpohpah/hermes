package presenter

import (
	"math"
	"sort"
	"strings"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

// CityCenterLngLat — Центральная площадь Владивостока, точка отсчёта
// «расстояния от центра» во фронтенде.
const CityCenterLngLat = 131.8855
const cityCenterLat = 43.1155

// haversineKm считает расстояние между точками в километрах.
func haversineKm(lng1, lat1, lng2, lat2 float64) float64 {
	const earthRadius = 6371.0
	toRad := func(deg float64) float64 { return deg * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)

	return 2 * earthRadius * math.Asin(math.Sqrt(a))
}

// DistanceFromCenterKm — расстояние по прямой от Центральной площади.
func DistanceFromCenterKm(lat, lon float64) float64 {
	return haversineKm(CityCenterLngLat, cityCenterLat, lon, lat)
}

// CatalogItem — объединённая view-model места или события для страниц,
// фрагментов и карты. Событие без своих координат наследует их от места.
type CatalogItem struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"` // place | event
	Scenario    string         `json:"scenario"`
	Category    string         `json:"category,omitempty"`
	Title       string         `json:"title"`
	Subtitle    string         `json:"subtitle,omitempty"`
	Short       string         `json:"short,omitempty"`
	Description string         `json:"description,omitempty"`
	District    string         `json:"district,omitempty"`
	Address     string         `json:"address,omitempty"`
	Lat         float64        `json:"lat"`
	Lon         float64        `json:"lon"`
	HasCoords   bool           `json:"has_coords"`
	Hours       string         `json:"hours,omitempty"`
	PriceFree   *bool          `json:"price_free,omitempty"`
	PriceLabel  string         `json:"price_label,omitempty"`
	Duration    string         `json:"duration,omitempty"`
	Season      string         `json:"season,omitempty"`
	Access      []string       `json:"access"`
	HowToGet    string         `json:"how_to_get,omitempty"`
	Facts       []string       `json:"facts"`
	Tags        []string       `json:"tags"`
	Photos      []models.Photo `json:"photos"`
	Beach       string         `json:"beach,omitempty"`
	SwimSeason  string         `json:"swim_season,omitempty"`
	DistanceKm  float64        `json:"distance_km"`
	Elevation   *int           `json:"elevation,omitempty"`
	Difficulty  *models.Difficulty `json:"difficulty,omitempty"`
	StartsAt    string         `json:"starts_at,omitempty"`
	EndsAt      string         `json:"ends_at,omitempty"`
	Venue       string         `json:"venue,omitempty"`
	Organizer   string         `json:"organizer,omitempty"`
	Age         string         `json:"age,omitempty"`
	PlaceID     string         `json:"place_id,omitempty"`
	PlaceName   string         `json:"place_name,omitempty"`
	LatestScore *ConditionAssessmentDTO `json:"latest_score,omitempty"`
}

// CatalogItemFromPlace строит view-model из места.
func CatalogItemFromPlace(in Place) CatalogItem {
	out := CatalogItem{
		ID:          in.ID,
		Type:        "place",
		Scenario:    string(in.Layer),
		Category:    in.Category,
		Title:       in.Name,
		Subtitle:    in.Subtitle,
		Short:       in.Short,
		Description: in.Description,
		District:    in.District,
		Address:     in.Address,
		Lat:         in.Lat,
		Lon:         in.Lon,
		HasCoords:   true,
		Hours:       in.Hours,
		PriceFree:   in.PriceFree,
		PriceLabel:  in.PriceLabel,
		Duration:    in.Duration,
		Season:      in.Season,
		Access:      in.Access,
		HowToGet:    in.HowToGet,
		Facts:       in.Facts,
		Tags:        in.Tags,
		Photos:      in.Photos,
		Beach:       in.Beach,
		SwimSeason:  in.SwimSeason,
		Elevation:   in.Elevation,
		Difficulty:  in.Difficulty,
		LatestScore: in.LatestScore,
	}
	out.DistanceKm = DistanceFromCenterKm(in.Lat, in.Lon)

	return out
}

// CatalogItemFromEvent строит view-model события; собственные координаты
// импортированного события берутся из него самого, иначе — от места проведения.
func CatalogItemFromEvent(in Event, place *Place) CatalogItem {
	out := CatalogItem{
		ID:          in.ID,
		Type:        "event",
		Category:    in.Category,
		Title:       in.Title,
		Short:       in.Short,
		Description: in.Description,
		Address:     in.Venue,
		StartsAt:    in.StartsAt,
		EndsAt:      in.EndsAt,
		Venue:       in.Venue,
		Organizer:   in.Organizer,
		Age:         in.Age,
		PlaceID:     in.PlaceID,
		PlaceName:   in.PlaceName,
		PriceFree:   in.PriceFree,
		PriceLabel:  in.PriceLabel,
	}
	if in.Lat != nil && in.Lon != nil {
		out.Lat = *in.Lat
		out.Lon = *in.Lon
		out.HasCoords = true
		out.Scenario = "city"
		out.DistanceKm = DistanceFromCenterKm(*in.Lat, *in.Lon)
	}
	if place != nil {
		out.Scenario = string(place.Layer)
		out.District = place.District
		out.Address = firstNonEmpty(in.Venue, place.Address)
		out.Lat = place.Lat
		out.Lon = place.Lon
		out.HasCoords = true
		out.Photos = place.Photos
		out.DistanceKm = DistanceFromCenterKm(place.Lat, place.Lon)
		if out.Scenario == "" {
			out.Scenario = "city"
		}
	}

	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}

// CatalogFilter — расширенные фильтры сценария из интерфейса (переключатели и
// чипы диалога фильтров). Применяются поверх выдачи use case.
type CatalogFilter struct {
	Free     bool
	Walkable bool    // пешком от центра: не дальше 2,5 км
	Kids     bool    // тег «с детьми»
	Distance float64 // 0 — любое, иначе максимум км от центра
	Difficulty []string
	NoCar    bool // пешком, транспорт или паром
	Beach    []string
	Swim     bool // есть купальный сезон
}

const walkableMaxKm = 2.5

// Matches проверяет один объект по набору фильтров.
func (f CatalogFilter) Matches(item CatalogItem) bool {
	if f.Free {
		free := item.PriceFree != nil && *item.PriceFree
		if !free && !strings.Contains(strings.ToLower(item.PriceLabel), "бесплат") {
			return false
		}
	}
	if f.Walkable && item.DistanceKm > walkableMaxKm {
		return false
	}
	if f.Kids && !hasTag(item.Tags, "с детьми") {
		return false
	}
	if f.Distance > 0 && item.DistanceKm > f.Distance {
		return false
	}
	if len(f.Difficulty) > 0 && !containsString(f.Difficulty, difficultyCode(item.Difficulty)) {
		return false
	}
	if f.NoCar && !accessWithoutCar(item.Access) {
		return false
	}
	if len(f.Beach) > 0 && !containsString(f.Beach, item.Beach) {
		return false
	}
	if f.Swim && strings.TrimSpace(item.SwimSeason) == "" {
		return false
	}

	return true
}

// difficultyCode переводит числовую сложность (1–5) в категории фильтра макета.
func difficultyCode(d *models.Difficulty) string {
	if d == nil {
		return ""
	}
	switch {
	case *d <= 2:
		return "easy"
	case *d == 3:
		return "medium"
	default:
		return "hard"
	}
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}

	return false
}

func hasTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if strings.EqualFold(strings.TrimSpace(tag), wanted) {
			return true
		}
	}

	return false
}

func accessWithoutCar(access []string) bool {
	for _, mode := range access {
		switch mode {
		case "walk", "transit", "ferry":
			return true
		}
	}

	return len(access) == 0
}

// SortCatalogItems сортирует выдачу справочника: recommended оставляет порядок
// репозитория, distance — ближе к центру, title — по алфавиту.
func SortCatalogItems(items []CatalogItem, sortBy string) {
	switch sortBy {
	case "distance":
		sort.SliceStable(items, func(i, j int) bool { return items[i].DistanceKm < items[j].DistanceKm })
	case "title":
		sort.SliceStable(items, func(i, j int) bool {
			return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
		})
	}
}
