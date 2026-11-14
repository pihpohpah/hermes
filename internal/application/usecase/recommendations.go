package usecase

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/pihpohpah/hermes/internal/domain/models"
)

const (
	// Лимиты ленты «Для вас» — как в макете (vlru-places/src/js/api/recommendations.js).
	mockRecPlaceLimit = 8
	mockRecEventLimit = 3
	mockRecRouteMax   = 2
)

// routeStopsByScenario — длина мок-маршрута для сценария (город длиннее:
// точки стоят плотнее). Зеркалит ROUTE_STOPS макета.
var routeStopsByScenario = map[models.LayerCode]int{
	models.LayerCity:   4,
	models.LayerNature: 3,
	models.LayerSea:    3,
}

var routeTitleByScenario = map[models.LayerCode]string{
	models.LayerCity:   "Прогулка по городу",
	models.LayerNature: "Выезд на природу",
	models.LayerSea:    "День у моря",
}

// RecommendedRoute — мок-маршрут ленты «Для вас»: точки подобраны жадно
// «каждая следующая — ближайшая к предыдущей», как в макете.
type RecommendedRoute struct {
	ID       string
	Title    string
	Scenario models.LayerCode
	Stops    []models.Place
}

// Recommendations — ответ мок-рекомендательной системы для ленты профиля.
type Recommendations struct {
	Places []models.Place
	Events []models.Event
	Routes []RecommendedRoute
}

// MOCK(рекомендательная система): это временная заглушка, а не рекомендации.
// Кандидаты берутся случайным образом: без скоринга, без фильтров и без учёта
// истории пользователя (просмотры, посещения и поиски не используются — лента
// не «подстраивается»). Персонализированные причины в подписях карточек
// намеренно не выдумываются, чтобы интерфейс не вводил в заблуждение.
// В продакшене ленту должен отдавать настоящий рекомендательный сервис
// (например, GET /me/recommendations), и этот use case заменяется на клиент к
// нему. Логика (лимиты, жадная сборка маршрутов) зеркалит recommendations.js
// дизайн-прототипа, чтобы переход на реальный сервис ничего не менял в UI.
type RecommendationsUseCase struct {
	places placeLister
	events eventLister
	now    func() time.Time
	rnd    *rand.Rand
}

// Интерфейсы placeLister и eventLister уже объявлены в places.go и events.go —
// здесь они переиспользуются.

func NewRecommendationsUseCase(places placeLister, events eventLister, now func() time.Time) *RecommendationsUseCase {
	return &RecommendationsUseCase{
		places: places,
		events: events,
		now:    now,
		rnd:    rand.New(rand.NewSource(now().UnixNano())), // MOCK: случайность вместо скоринга.
	}
}

func (u *RecommendationsUseCase) Do(ctx context.Context) (Recommendations, error) {
	places, err := u.places.List(ctx, models.PlaceFilter{Limit: 500})
	if err != nil {
		return Recommendations{}, fmt.Errorf("list places for recommendations: %w", err)
	}
	events, err := u.events.List(ctx, models.EventFilter{UpcomingOnly: true, Limit: 200, Now: u.now()})
	if err != nil {
		return Recommendations{}, fmt.Errorf("list events for recommendations: %w", err)
	}

	u.rnd.Shuffle(len(places), func(i, j int) { places[i], places[j] = places[j], places[i] })

	recs := Recommendations{
		Places: places[:min(len(places), mockRecPlaceLimit)],
	}
	if len(events) > 0 {
		u.rnd.Shuffle(len(events), func(i, j int) { events[i], events[j] = events[j], events[i] })
		recs.Events = events[:min(len(events), mockRecEventLimit)]
	}
	recs.Routes = u.mockRoutes(places)

	return recs, nil
}

// mockRoutes собирает до mockRecRouteMax маршрутов: для каждого сценария с
// достаточным числом точек — жадная цепочка ближайших соседей.
func (u *RecommendationsUseCase) mockRoutes(shuffled []models.Place) []RecommendedRoute {
	byScenario := map[models.LayerCode][]models.Place{}
	for _, place := range shuffled {
		byScenario[place.Layer] = append(byScenario[place.Layer], place)
	}

	routes := make([]RecommendedRoute, 0, mockRecRouteMax)
	for _, scenario := range []models.LayerCode{models.LayerCity, models.LayerNature, models.LayerSea} {
		if len(routes) >= mockRecRouteMax {
			break
		}
		stopsWanted := routeStopsByScenario[scenario]
		pool := byScenario[scenario]
		if len(pool) < stopsWanted {
			continue
		}

		stops := []models.Place{pool[0]}
		pool = pool[1:]
		for len(stops) < stopsWanted && len(pool) > 0 {
			last := stops[len(stops)-1]
			nearest := 0
			nearestKm := haversineKm(last.Location, pool[0].Location)
			for i := 1; i < len(pool); i++ {
				if km := haversineKm(last.Location, pool[i].Location); km < nearestKm {
					nearest, nearestKm = i, km
				}
			}
			stops = append(stops, pool[nearest])
			pool = append(pool[:nearest], pool[nearest+1:]...)
		}

		routes = append(routes, RecommendedRoute{
			ID:       "mock-route-" + string(scenario),
			Title:    routeTitleByScenario[scenario],
			Scenario: scenario,
			Stops:    stops,
		})
	}

	return routes
}

// haversineKm считает расстояние между точками в километрах.
func haversineKm(a, b models.Point) float64 {
	const earthRadius = 6371.0
	toRad := func(deg float64) float64 { return deg * math.Pi / 180 }
	dLat := toRad(b.Lat - a.Lat)
	dLng := toRad(b.Lon - a.Lon)
	sin := func(v float64) float64 { return math.Sin(v / 2) }

	d := sin(dLat)*sin(dLat) +
		math.Cos(toRad(a.Lat))*math.Cos(toRad(b.Lat))*sin(dLng)*sin(dLng)

	return 2 * earthRadius * math.Asin(math.Sqrt(d))
}
