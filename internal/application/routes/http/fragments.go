package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/domain/models"
	"github.com/pihpohpah/hermes/web/templates"
)

// Fragments отдаёт HTML-части для htmx-свопов. Каждый фрагмент строится из
// тех же query-параметров, что и страница, поэтому URL остаётся единственным
// источником состояния экрана.
type Fragments struct {
	logger *slog.Logger
	pages  *Pages
}

func NewFragments(logger *slog.Logger, pages *Pages) *Fragments {
	return &Fragments{logger: logger, pages: pages}
}

func (f *Fragments) render(w http.ResponseWriter, r *http.Request, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		f.logger.Error("render fragment failed", slog.Any("error", err))
	}
}

func (f *Fragments) error(w http.ResponseWriter, _ *http.Request, what string, err error) {
	f.logger.Error(what+" failed", slog.Any("error", err))
	http.Error(w, "internal error", statusCode(err))
}

// GET /fragments/map-panel?s=&q=&cat=&type=&place=&panel=&tab= — внутренности
// панели карты: шапка (поиск, чипы, тулбар), тело (фильтры, список, превью,
// редактор маршрута), подвал.
func (f *Fragments) MapPanel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	view, err := f.pages.loadMapView(r.Context(), r, parseMapQuery(r))
	if err != nil {
		f.error(w, r, "map panel fragment", err)

		return
	}

	f.render(w, r, templates.MapPanelInner(view))
}

// GET /fragments/catalog-results?... — сетка или список результатов справочника.
func (f *Fragments) CatalogResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	view, err := f.pages.loadCatalogView(r.Context(), r, parseCatalogQuery(r))
	if err != nil {
		f.error(w, r, "catalog results fragment", err)

		return
	}

	f.render(w, r, templates.CatalogResults(view))
}

// GET /fragments/pins?kind= — список быстрых меток для панели «Сейчас».
func (f *Fragments) Pins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	state := parseNowQuery(r)
	pins, err := f.pages.loadPins(r.Context(), r, state.Kind)
	if err != nil {
		f.error(w, r, "pins fragment", err)

		return
	}

	f.render(w, r, templates.NowPanelInner(templates.NowView{
		User: f.pages.viewUser(r),
		Kind: state.Kind,
		Pin:  state.Pin,
		Pins: presenter.PinsToHTTP(pins),
	}))
}

// GET /fragments/pin/{id} — превью одной метки.
func (f *Fragments) Pin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	state := parseNowQuery(r)
	pin, err := f.pages.getPin.Do(r.Context(), r.PathValue("id"), viewerID(r.Context()))
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "pin not found", http.StatusNotFound)

			return
		}
		f.error(w, r, "pin fragment", err)

		return
	}

	// Чипы типов в шапке показывают счётчики, поэтому подтягиваем весь список.
	pins, listErr := f.pages.loadPins(r.Context(), r, state.Kind)
	if listErr != nil {
		f.error(w, r, "pin fragment", listErr)

		return
	}

	item := presenter.PinToHTTP(pin)
	f.render(w, r, templates.NowPanelInner(templates.NowView{
		User:     f.pages.viewUser(r),
		Kind:     state.Kind,
		Pin:      state.Pin,
		Pins:     presenter.PinsToHTTP(pins),
		LivePins: len(pins),
		Selected: &item,
	}))
}

// GET /fragments/profile-tab?tab= — содержимое вкладки профиля.
func (f *Fragments) ProfileTab(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	user := f.pages.viewUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)

		return
	}

	view, err := f.pages.loadProfileView(r.Context(), r, user, parseProfileTab(r))
	if err != nil {
		f.error(w, r, "profile tab fragment", err)

		return
	}

	f.render(w, r, templates.ProfileTab(view))
}

// GET /fragments/live-count — число активных меток для шапки и чипа на карте.
func (f *Fragments) LiveCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	f.render(w, r, templates.LiveCount(f.pages.countLivePins(r.Context())))
}

// MapItems отдаёт объединённую выдачу мест и событий для клиентской карты:
// маркеры, кластеры и fitTo считаются во фронтенде, сервер даёт данные.
type MapItems struct {
	logger *slog.Logger
	pages  *Pages
}

func NewMapItems(logger *slog.Logger, pages *Pages) *MapItems {
	return &MapItems{logger: logger, pages: pages}
}

// GET /api/map-items?s=&q=&cat=&type= + фильтры — {success, data:{items, type_counts, category_counts}}.
func (h *MapItems) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	state := parseMapQuery(r)
	result, err := h.pages.loader.loadItems(r.Context(), catalogQuery{
		Scenario: state.Scenario, Q: state.Q, Category: state.Category,
		Type: state.Type, Filters: state.Filters,
	})
	if err != nil {
		NewAPIResponseWriter[mapItemsResponse](h.logger).Error(w, statusCode(err), err)

		return
	}

	NewAPIResponseWriter[mapItemsResponse](h.logger).Success(w, mapItemsResponse{
		Items:          result.Items,
		TypeCounts:     result.TypeCounts,
		CategoryCounts: result.CategoryCounts,
	})
}

type mapItemsResponse struct {
	Items          []presenter.CatalogItem `json:"items"`
	TypeCounts     map[string]int          `json:"type_counts"`
	CategoryCounts map[string]int          `json:"category_counts"`
}

// Item отдаёт один объект (место или событие) для клиентского маршрута:
// GET /api/items/{id} → {success, data: CatalogItem}.
type Item struct {
	logger *slog.Logger
	pages  *Pages
}

func NewItem(logger *slog.Logger, pages *Pages) *Item {
	return &Item{logger: logger, pages: pages}
}

func (h *Item) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	item, err := h.pages.loadItem(r.Context(), r.PathValue("id"))
	if err != nil {
		NewAPIResponseWriter[presenter.CatalogItem](h.logger).Error(w, statusCode(err), err)

		return
	}

	NewAPIResponseWriter[presenter.CatalogItem](h.logger).Success(w, item)
}
