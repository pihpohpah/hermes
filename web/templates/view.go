package templates

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/pihpohpah/hermes/internal/application/presenter"
)

// View-структуры страниц и фрагментов. Состояние экрана — в URL, поэтому
// каждая view несёт Query, а хелперы строят ссылки и htmx-адреса.

type MapQueryView struct {
	Scenario string
	Q        string
	Category string
	Type     string
	Selected string
	Panel    string
	Tab      string
	Filters  presenter.CatalogFilter
}

type MapView struct {
	User           *presenter.User
	LivePins       int
	Query          MapQueryView
	Items          []presenter.CatalogItem
	TypeCounts     map[string]int
	CategoryCounts map[string]int
	ActiveFilters  int
	Selected       *presenter.CatalogItem
}

type NowView struct {
	User     *presenter.User
	LivePins int
	Kind     string
	Pin      string
	Create   string
	Pins     []presenter.Pin
	Selected *presenter.Pin
}

type PinsView struct {
	Kind     string
	Pin      string
	Pins     []presenter.Pin
	User     *presenter.User
	LivePins int
}

type PinView struct {
	Pin  *presenter.Pin
	User *presenter.User
	Kind string // текущий фильтр типа — для ссылки «Все метки»
}

type CatalogQueryView struct {
	Scenario   string
	Q          string
	Category   string
	Collection string
	Type       string
	Sort       string
	View       string
	Filters    presenter.CatalogFilter
}

type CollectionCard struct {
	ID      string
	Title   string
	Count   int
	Cover   *presenter.CatalogItem
	Pressed bool
}

type CatalogView struct {
	User          *presenter.User
	Query         CatalogQueryView
	Items         []presenter.CatalogItem
	Collections   []CollectionCard
	ActiveFilters int
}

type DetailView struct {
	User       *presenter.User
	Item       presenter.CatalogItem
	Similar    []presenter.CatalogItem
	EventsHere []presenter.CatalogItem
}

type AddView struct {
	User     *presenter.User
	Type     string
	FixID    string
	FixItem  *presenter.CatalogItem
	Scenario string
}

type AddSuccessView struct {
	User *presenter.User
	Type string
}

type ProfileView struct {
	User        *presenter.User
	Tab         string
	Pins        []presenter.Pin
	Submissions []presenter.Submission
	Trips       []presenter.Trip
	// ForYou — лента «Для вас»; заполняется только для вкладки foryou.
	// MOCK(рекомендательная система): сейчас это случайная подборка,
	// а не персональные рекомендации.
	ForYou *ForYouView
}

// ForYouView — лента «Для вас» профиля (вкладка foryou).
type ForYouView struct {
	Places []presenter.CatalogItem
	Events []presenter.CatalogItem
	Routes []ForYouRoute
}

// ForYouRoute — мок-маршрут ленты «Для вас»: точки подобраны жадно
// «ближайшая к предыдущей», без маршрутизации. MOCK.
type ForYouRoute struct {
	ID       string
	Title    string
	Scenario string // city | nature | sea
	Stops    []presenter.CatalogItem
}

type RulesView struct {
	User    *presenter.User
	Section string
}

type LayoutView struct {
	Title        string
	Section      string // map | catalog | now | ""
	User         *presenter.User
	LivePins     int
	Scenario     string // активный сценарий переключателя (пусто — без переключателя)
	ScenarioHref func(string) string
	WithMap      bool // страницы с картой подключают MapLibre и map.js
}

func (l LayoutView) scenarioHref(id string) string {
	if l.ScenarioHref != nil {
		return l.ScenarioHref(id)
	}

	return "/?s=" + id
}

const pageSize = 12

// ── URL-хелперы карты ──

func mapValues(q MapQueryView) url.Values {
	values := url.Values{}
	values.Set("s", q.Scenario)
	if q.Q != "" {
		values.Set("q", q.Q)
	}
	if q.Category != "" {
		values.Set("cat", q.Category)
	}
	if q.Type != "all" && q.Type != "" {
		values.Set("type", q.Type)
	}
	if q.Selected != "" {
		values.Set("place", q.Selected)
	}
	if q.Panel != "" {
		values.Set("panel", q.Panel)
	}
	if q.Tab != "" && q.Tab != "filters" {
		values.Set("tab", q.Tab)
	}
	applyFilterValues(values, q.Filters)

	return values
}

func applyFilterValues(values url.Values, f presenter.CatalogFilter) {
	if f.Free {
		values.Set("free", "1")
	}
	if f.Walkable {
		values.Set("walkable", "1")
	}
	if f.Kids {
		values.Set("kids", "1")
	}
	if f.NoCar {
		values.Set("noCar", "1")
	}
	if f.Swim {
		values.Set("swim", "1")
	}
	if f.Distance > 0 {
		values.Set("distance", strconv.FormatFloat(f.Distance, 'f', -1, 64))
	}
	if len(f.Difficulty) > 0 {
		values.Set("difficulty", strings.Join(f.Difficulty, ","))
	}
	if len(f.Beach) > 0 {
		values.Set("beach", strings.Join(f.Beach, ","))
	}
}

func (q MapQueryView) pageURL() string {
	return "/?" + mapValues(q).Encode()
}

// activeChip — съёмный чип активного фильтра: подпись и ссылки без этого условия.
type activeChip struct {
	Label    string
	Page     string
	Fragment string
}

// mapActiveChips собирает чипы активных фильтров сценария для строки над списком.
func mapActiveChips(q MapQueryView) []activeChip {
	chips := []activeChip{}
	for _, spec := range presenter.Filters[q.Scenario] {
		f := q.Filters
		switch spec.ID {
		case "free":
			if f.Free {
				next := q
				next.Filters.Free = false
				chips = append(chips, chipFor(spec.Label, next))
			}
		case "walkable":
			if f.Walkable {
				next := q
				next.Filters.Walkable = false
				chips = append(chips, chipFor(spec.Label, next))
			}
		case "kids":
			if f.Kids {
				next := q
				next.Filters.Kids = false
				chips = append(chips, chipFor(spec.Label, next))
			}
		case "noCar":
			if f.NoCar {
				next := q
				next.Filters.NoCar = false
				chips = append(chips, chipFor(spec.Label, next))
			}
		case "swim":
			if f.Swim {
				next := q
				next.Filters.Swim = false
				chips = append(chips, chipFor(spec.Label, next))
			}
		case "distance":
			if f.Distance > 0 {
				label := strconv.FormatFloat(f.Distance, 'f', -1, 64)
				for _, option := range spec.Options {
					if option.Value == label {
						label = option.Label
					}
				}
				next := q
				next.Filters.Distance = 0
				chips = append(chips, chipFor(label, next))
			}
		case "difficulty":
			for _, value := range f.Difficulty {
				label := value
				for _, option := range spec.Options {
					if option.Value == value {
						label = option.Label
					}
				}
				next := q
				next.Filters.Difficulty = withoutString(f.Difficulty, value)
				chips = append(chips, chipFor(label, next))
			}
		case "beach":
			for _, value := range f.Beach {
				label := value
				for _, option := range spec.Options {
					if option.Value == value {
						label = option.Label
					}
				}
				next := q
				next.Filters.Beach = withoutString(f.Beach, value)
				chips = append(chips, chipFor(label, next))
			}
		}
	}

	return chips
}

func chipFor(label string, q MapQueryView) activeChip {
	return activeChip{Label: label, Page: q.pageURL(), Fragment: q.fragmentURL()}
}

func withoutString(list []string, value string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		if item != value {
			out = append(out, item)
		}
	}

	return out
}

func (q MapQueryView) fragmentURL() string {
	return "/fragments/map-panel?" + mapValues(q).Encode()
}

// with возвращает копию запроса с изменёнными полями.
func (q MapQueryView) with(mutate func(*MapQueryView)) MapQueryView {
	next := q
	mutate(&next)

	return next
}

// ── URL-хелперы справочника ──

func catalogValues(q CatalogQueryView) url.Values {
	values := url.Values{}
	values.Set("s", q.Scenario)
	if q.Q != "" {
		values.Set("q", q.Q)
	}
	if q.Category != "" {
		values.Set("cat", q.Category)
	}
	if q.Collection != "" {
		values.Set("col", q.Collection)
	}
	if q.Type != "all" && q.Type != "" {
		values.Set("type", q.Type)
	}
	if q.Sort != "" && q.Sort != "recommended" {
		values.Set("sort", q.Sort)
	}
	if q.View == "list" {
		values.Set("view", "list")
	}
	applyFilterValues(values, q.Filters)

	return values
}

func (q CatalogQueryView) pageURL() string {
	return "/catalog?" + catalogValues(q).Encode()
}

func (q CatalogQueryView) fragmentURL() string {
	return "/fragments/catalog-results?" + catalogValues(q).Encode()
}

func (q CatalogQueryView) with(mutate func(*CatalogQueryView)) CatalogQueryView {
	next := q
	mutate(&next)

	return next
}

// catalogActiveChips — съёмные чипы активных фильтров справочника.
func catalogActiveChips(q CatalogQueryView) []activeChip {
	chips := []activeChip{}
	for _, spec := range presenter.Filters[q.Scenario] {
		f := q.Filters
		switch spec.ID {
		case "free":
			if f.Free {
				next := q
				next.Filters.Free = false
				chips = append(chips, catalogChipFor(spec.Label, next))
			}
		case "walkable":
			if f.Walkable {
				next := q
				next.Filters.Walkable = false
				chips = append(chips, catalogChipFor(spec.Label, next))
			}
		case "kids":
			if f.Kids {
				next := q
				next.Filters.Kids = false
				chips = append(chips, catalogChipFor(spec.Label, next))
			}
		case "noCar":
			if f.NoCar {
				next := q
				next.Filters.NoCar = false
				chips = append(chips, catalogChipFor(spec.Label, next))
			}
		case "swim":
			if f.Swim {
				next := q
				next.Filters.Swim = false
				chips = append(chips, catalogChipFor(spec.Label, next))
			}
		case "distance":
			if f.Distance > 0 {
				label := strconv.FormatFloat(f.Distance, 'f', -1, 64)
				for _, option := range spec.Options {
					if option.Value == label {
						label = option.Label
					}
				}
				next := q
				next.Filters.Distance = 0
				chips = append(chips, catalogChipFor(label, next))
			}
		case "difficulty":
			for _, value := range f.Difficulty {
				label := value
				for _, option := range spec.Options {
					if option.Value == value {
						label = option.Label
					}
				}
				next := q
				next.Filters.Difficulty = withoutString(f.Difficulty, value)
				chips = append(chips, catalogChipFor(label, next))
			}
		case "beach":
			for _, value := range f.Beach {
				label := value
				for _, option := range spec.Options {
					if option.Value == value {
						label = option.Label
					}
				}
				next := q
				next.Filters.Beach = withoutString(f.Beach, value)
				chips = append(chips, catalogChipFor(label, next))
			}
		}
	}

	return chips
}

func catalogChipFor(label string, q CatalogQueryView) activeChip {
	return activeChip{Label: label, Page: q.pageURL(), Fragment: q.fragmentURL()}
}

// nowURL строит адрес экрана «Сейчас».
func nowURL(kind, pin, create string) string {
	values := url.Values{}
	if kind != "" {
		values.Set("kind", kind)
	}
	if pin != "" {
		values.Set("pin", pin)
	}
	if create != "" {
		values.Set("create", create)
	}
	if len(values) == 0 {
		return "/now"
	}

	return "/now?" + values.Encode()
}

func pinsFragmentURL(kind, pin string) string {
	values := url.Values{}
	if kind != "" {
		values.Set("kind", kind)
	}

	return "/fragments/pins?" + values.Encode()
}

// ── Подписи и форматирование для шаблонов ──

func itemURL(item presenter.CatalogItem) string {
	if item.Type == "event" {
		return "/event/" + item.ID
	}

	return "/place/" + item.ID
}

func itemScenario(item presenter.CatalogItem) string {
	if item.Type == "event" {
		return "timed"
	}

	return item.Scenario
}
