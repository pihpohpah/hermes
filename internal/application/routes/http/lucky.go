package http

import (
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"slices"
	"strconv"

	"github.com/pihpohpah/hermes/internal/application/presenter"
)

// Lucky — «Мне повезёт»: серверный бросок случайного места, события или
// маршрута. Маршрут строится жадно от случайной стартовой точки: каждая
// следующая — ближайшая к предыдущей, из трёх ближайших предпочитается
// другая категория, чтобы прогулка не состояла из одних музеев.
type Lucky struct {
	logger *slog.Logger
	pages  *Pages
}

func NewLucky(logger *slog.Logger, pages *Pages) *Lucky {
	return &Lucky{logger: logger, pages: pages}
}

// GET /api/lucky?type=any|place|event|route&s=any|city|nature|sea&stops=2..6&exclude=<id>
// → {success, data: {type: "place"|"event", item} | {type: "route", stops: [{item, from_previous_km}]}}.
// Пустой результат — data: null (во всех сценариях мало точек и т.п.).
func (h *Lucky) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	query := r.URL.Query()
	kind := query.Get("type")
	if !slices.Contains([]string{"any", "place", "event", "route"}, kind) {
		kind = "any"
	}

	stops, err := strconv.Atoi(query.Get("stops"))
	if err != nil || stops < 2 {
		stops = 3
	}
	if stops > 6 {
		stops = 6
	}

	exclude := query.Get("exclude")

	pool, err := h.pool(r, query.Get("s"))
	if err != nil {
		NewAPIResponseWriter[any](h.logger).Error(w, statusCode(err), err)

		return
	}

	places := filterCoords(filterType(pool, "place"))
	events := filterType(pool, "event")

	available := map[string]bool{
		"place": len(places) > 0,
		"event": len(events) > 0,
		"route": len(places) >= 2,
	}

	resolved := kind
	if resolved == "any" {
		var options []string
		for _, option := range []string{"place", "event", "route"} {
			if available[option] {
				options = append(options, option)
			}
		}
		if len(options) == 0 {
			NewAPIResponseWriter[any](h.logger).Success(w, nil)

			return
		}
		resolved = options[rand.IntN(len(options))]
	}
	if !available[resolved] {
		NewAPIResponseWriter[any](h.logger).Success(w, nil)

		return
	}

	switch resolved {
	case "route":
		NewAPIResponseWriter[any](h.logger).Success(w, luckyRoute{
			Type:  "route",
			Stops: buildLuckyRoute(places, stops),
		})

		return
	default:
		list := places
		if resolved == "event" {
			list = events
		}
		fresh := slices.DeleteFunc(slices.Clone(list), func(item presenter.CatalogItem) bool {
			return len(list) > 1 && item.ID == exclude
		})
		NewAPIResponseWriter[any](h.logger).Success(w, luckyItem{
			Type: resolved,
			Item: fresh[rand.IntN(len(fresh))],
		})
	}
}

// luckyItem / luckyRoute — тело ответа: одно место/событие или готовый маршрут.
type luckyItem struct {
	Type string                `json:"type"`
	Item presenter.CatalogItem `json:"item"`
}

type luckyStop struct {
	Item           presenter.CatalogItem `json:"item"`
	FromPreviousKm float64               `json:"from_previous_km"`
}

type luckyRoute struct {
	Type  string      `json:"type"`
	Stops []luckyStop `json:"stops"`
}

// pool собирает выдачу для броска. Для s=any мерджит три сценария: фильтр
// слоя в loadItems строгий, а события слоя не знают — дедупликация по ID.
func (h *Lucky) pool(r *http.Request, scenario string) ([]presenter.CatalogItem, error) {
	scenarios := []string{scenarioCity, scenarioNature, scenarioSea}
	if scenario != "" && scenario != "any" {
		scenarios = []string{validScenario(scenario)}
	}

	seen := map[string]bool{}
	var items []presenter.CatalogItem
	for _, s := range scenarios {
		result, err := h.pages.loader.loadItems(r.Context(), catalogQuery{Scenario: s, Type: "all"})
		if err != nil {
			return nil, err
		}
		for _, item := range result.Items {
			if !seen[item.ID] {
				seen[item.ID] = true
				items = append(items, item)
			}
		}
	}

	return items, nil
}

func filterType(items []presenter.CatalogItem, itemType string) []presenter.CatalogItem {
	return slices.DeleteFunc(slices.Clone(items), func(item presenter.CatalogItem) bool {
		return item.Type != itemType
	})
}

func filterCoords(items []presenter.CatalogItem) []presenter.CatalogItem {
	return slices.DeleteFunc(slices.Clone(items), func(item presenter.CatalogItem) bool {
		return !item.HasCoords
	})
}

func buildLuckyRoute(places []presenter.CatalogItem, size int) []luckyStop {
	pool := slices.Clone(places)
	stops := []presenter.CatalogItem{pool[rand.IntN(len(pool))]}
	pool = slices.DeleteFunc(pool, func(item presenter.CatalogItem) bool { return item.ID == stops[0].ID })

	for len(stops) < size && len(pool) > 0 {
		last := stops[len(stops)-1]
		slices.SortFunc(pool, func(a, b presenter.CatalogItem) int {
			return int((luckyKm(last, a) - luckyKm(last, b)) * 1000)
		})
		nearest := pool[:min(3, len(pool))]
		next := nearest[0]
		for _, candidate := range nearest {
			if candidate.Category != last.Category {
				next = candidate

				break
			}
		}
		stops = append(stops, next)
		pool = slices.DeleteFunc(pool, func(item presenter.CatalogItem) bool { return item.ID == next.ID })
	}

	result := make([]luckyStop, 0, len(stops))
	for i, stop := range stops {
		leg := 0.0
		if i > 0 {
			leg = luckyKm(stops[i-1], stop)
		}
		result = append(result, luckyStop{Item: stop, FromPreviousKm: leg})
	}

	return result
}

func luckyKm(a, b presenter.CatalogItem) float64 {
	const rad = math.Pi / 180
	dLat := (b.Lat - a.Lat) * rad
	dLon := (b.Lon - a.Lon) * rad
	s := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(a.Lat*rad)*math.Cos(b.Lat*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)

	return 2 * 6371 * math.Asin(math.Sqrt(s))
}
