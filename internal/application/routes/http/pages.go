package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/domain/models"
	"github.com/pihpohpah/hermes/web/templates"
)

// Pages собирает серверно-рендеренные экраны продукта «Места VL.ru».
// Каждый handler рендерит полную страницу; динамические части обновляются
// htmx-фрагментами (см. fragments.go), состояние экрана живёт в URL.
type Pages struct {
	logger *slog.Logger
	loader catalogLoader

	getPlace        GetPlaceUC
	getEvent        GetEventUC
	listSimilar     ListSimilarPlacesUC
	listEventsAt    ListEventsUC
	listPins        ListPinsUC
	getPin          GetPinUC
	listCollections ListCollectionsUC
	listSubmissions ListSubmissionsUC
}

func NewPages(
	logger *slog.Logger,
	loader catalogLoader,
	getPlace GetPlaceUC,
	getEvent GetEventUC,
	listSimilar ListSimilarPlacesUC,
	listEventsAt ListEventsUC,
	listPins ListPinsUC,
	getPin GetPinUC,
	listCollections ListCollectionsUC,
	listSubmissions ListSubmissionsUC,
) *Pages {
	return &Pages{
		logger:          logger,
		loader:          loader,
		getPlace:        getPlace,
		getEvent:        getEvent,
		listSimilar:     listSimilar,
		listEventsAt:    listEventsAt,
		listPins:        listPins,
		getPin:          getPin,
		listCollections: listCollections,
		listSubmissions: listSubmissions,
	}
}

func (p *Pages) viewUser(r *http.Request) *presenter.User {
	auth, ok := AuthFromContext(r.Context())
	if !ok {
		return nil
	}

	user := presenter.UserToHTTP(auth.User)

	return &user
}

// loadMapView собирает данные экрана карты из query-строки.
func (p *Pages) loadMapView(ctx context.Context, r *http.Request, state mapQuery) (templates.MapView, error) {
	result, err := p.loader.loadItems(ctx, catalogQuery{
		Scenario: state.Scenario, Q: state.Q, Category: state.Category,
		Type: state.Type, Filters: state.Filters,
	})
	if err != nil {
		return templates.MapView{}, err
	}

	// Счётчики для сегмента «Всё/Места/События» считаются без фильтра типа.
	counts := result.TypeCounts
	countState := catalogQuery{
		Scenario: state.Scenario, Q: state.Q, Category: state.Category,
		Type: "all", Filters: state.Filters,
	}
	if counted, err := p.loader.loadItems(ctx, countState); err == nil {
		counts = counted.TypeCounts
	}

	view := templates.MapView{
		User:           p.viewUser(r),
		Query:          templates.MapQueryView(state),
		Items:          result.Items,
		TypeCounts:     counts,
		CategoryCounts: result.CategoryCounts,
		ActiveFilters:  hasActiveFilters(state.Filters),
	}

	if state.Selected != "" {
		selected, err := p.loadItem(ctx, state.Selected)
		if err != nil && !errors.Is(err, models.ErrNotFound) {
			return templates.MapView{}, err
		}
		if err == nil && (selected.Scenario == state.Scenario || state.Type == "event") {
			view.Selected = &selected
		}
	}

	return view, nil
}

// loadItem достаёт место или событие по id (сначала место, потом событие).
func (p *Pages) loadItem(ctx context.Context, id string) (presenter.CatalogItem, error) {
	place, err := p.getPlace.Do(ctx, id)
	if err == nil {
		return presenter.CatalogItemFromPlace(presenter.PlaceToHTTP(place)), nil
	}
	if !errors.Is(err, models.ErrNotFound) {
		return presenter.CatalogItem{}, err
	}

	event, err := p.getEvent.Do(ctx, id)
	if err != nil {
		return presenter.CatalogItem{}, err
	}

	var venue *presenter.Place
	if event.PlaceID != nil {
		if placeModel, placeErr := p.getPlace.Do(ctx, *event.PlaceID); placeErr == nil {
			item := presenter.PlaceToHTTP(placeModel)
			venue = &item
		}
	}

	return presenter.CatalogItemFromEvent(presenter.EventToHTTP(event), venue), nil
}

// GET / — карта: сценарии, поиск, категории, фильтры, список и превью.
func (p *Pages) MapPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	view, err := p.loadMapView(r.Context(), r, parseMapQuery(r))
	if err != nil {
		p.pageError(w, r, "map page", err)

		return
	}
	view.LivePins = p.countLivePins(r.Context())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	p.render(w, r, templates.MapPage(view))
}

// GET /now — быстрые метки жителей поверх той же карты.
func (p *Pages) NowPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	state := parseNowQuery(r)
	pins, err := p.loadPins(r.Context(), r, state.Kind)
	if err != nil {
		p.pageError(w, r, "now page", err)

		return
	}

	view := templates.NowView{
		User:     p.viewUser(r),
		Kind:     state.Kind,
		Pin:      state.Pin,
		Create:   state.Create,
		Pins:     presenter.PinsToHTTP(pins),
		LivePins: len(pins),
	}
	if state.Pin != "" {
		pin, err := p.getPin.Do(r.Context(), state.Pin, viewerID(r.Context()))
		if err != nil && !errors.Is(err, models.ErrNotFound) {
			p.pageError(w, r, "now page", err)

			return
		}
		if err == nil {
			item := presenter.PinToHTTP(pin)
			view.Selected = &item
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	p.render(w, r, templates.NowPage(view))
}

type nowQuery struct {
	Kind   string
	Pin    string
	Create string // place | details
}

func parseNowQuery(r *http.Request) nowQuery {
	query := r.URL.Query()
	create := query.Get("create")
	if create != "place" && create != "details" {
		create = ""
	}

	return nowQuery{
		Kind:   query.Get("kind"),
		Pin:    query.Get("pin"),
		Create: create,
	}
}

func (p *Pages) loadPins(ctx context.Context, r *http.Request, kind string) ([]models.Pin, error) {
	filter := models.PinFilter{Limit: 200}
	if kind != "" {
		parsed, err := models.NewPinKind(kind)
		if err != nil {
			return nil, err
		}
		filter.Kind = &parsed
	}
	filter.Viewer = viewerID(r.Context())

	return p.listPins.Do(ctx, filter)
}

func (p *Pages) countLivePins(ctx context.Context) int {
	pins, err := p.listPins.Do(ctx, models.PinFilter{Limit: 1})
	if err != nil {
		return 0
	}

	return len(pins)
}

// GET /catalog — справочник: поиск, подборки, сортировка, сетка или список.
func (p *Pages) CatalogPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	view, err := p.loadCatalogView(r.Context(), r, parseCatalogQuery(r))
	if err != nil {
		p.pageError(w, r, "catalog page", err)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	p.render(w, r, templates.CatalogPage(view))
}

func (p *Pages) loadCatalogView(ctx context.Context, r *http.Request, state catalogQuery) (templates.CatalogView, error) {
	result, err := p.loader.loadItems(ctx, state)
	if err != nil {
		return templates.CatalogView{}, err
	}

	collections, err := p.listCollections.Do(ctx, nil)
	if err != nil {
		return templates.CatalogView{}, err
	}

	cards := make([]templates.CollectionCard, 0, len(collections))
	for _, collection := range collections {
		item := presenter.CollectionToHTTP(collection)
		count := 0
		if detail, err := p.loader.collections.Do(ctx, collection.ID); err == nil {
			count = len(detail.Items)
		}
		var cover *presenter.CatalogItem
		if item.Cover != "" {
			if coverItem, err := p.loadItem(ctx, item.Cover); err == nil {
				cover = &coverItem
			}
		}
		cards = append(cards, templates.CollectionCard{
			ID:      item.ID,
			Title:   item.Title,
			Count:   count,
			Cover:   cover,
			Pressed: state.Collection == item.ID,
		})
	}

	return templates.CatalogView{
		User:          p.viewUser(r),
		Query:         templates.CatalogQueryView(state),
		Items:         result.Items,
		Collections:   cards,
		ActiveFilters: hasActiveFilters(state.Filters),
	}, nil
}

// GET /place/{id} — карточка места.
func (p *Pages) PlacePage(w http.ResponseWriter, r *http.Request) {
	p.detailPage(w, r, false)
}

// GET /event/{id} — карточка события.
func (p *Pages) EventPage(w http.ResponseWriter, r *http.Request) {
	p.detailPage(w, r, true)
}

func (p *Pages) detailPage(w http.ResponseWriter, r *http.Request, wantEvent bool) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	ctx := r.Context()
	id := r.PathValue("id")

	item, err := p.loadItem(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			p.NotFoundPage(w, r)

			return
		}
		p.pageError(w, r, "detail page", err)

		return
	}

	if (item.Type == "event") != wantEvent {
		if item.Type == "event" {
			http.Redirect(w, r, "/event/"+item.ID, http.StatusPermanentRedirect)
		} else {
			http.Redirect(w, r, "/place/"+item.ID, http.StatusPermanentRedirect)
		}

		return
	}

	similarPlaces, err := p.listSimilar.Do(ctx, id, 6)
	if err != nil && !errors.Is(err, models.ErrNotFound) {
		p.pageError(w, r, "detail page", err)

		return
	}
	similar := make([]presenter.CatalogItem, 0, len(similarPlaces))
	for _, place := range similarPlaces {
		similar = append(similar, presenter.CatalogItemFromPlace(presenter.PlaceToHTTP(place)))
	}

	var eventsHere []presenter.CatalogItem
	if item.Type == "place" {
		events, err := p.listEventsAt.Do(ctx, models.EventFilter{PlaceID: &id, UpcomingOnly: true, Limit: 10})
		if err != nil {
			p.pageError(w, r, "detail page", err)

			return
		}
		for _, event := range events {
			eventsHere = append(eventsHere, presenter.CatalogItemFromEvent(presenter.EventToHTTP(event), nil))
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	p.render(w, r, templates.DetailPage(templates.DetailView{
		User:       p.viewUser(r),
		Item:       item,
		Similar:    similar,
		EventsHere: eventsHere,
	}))
}

// GET /add — мастер добавления места, события или правки.
func (p *Pages) AddPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	query := r.URL.Query()
	fixID := query.Get("fix")
	view := templates.AddView{
		User:     p.viewUser(r),
		Type:     query.Get("type"),
		FixID:    fixID,
		Scenario: validScenario(query.Get("scenario")),
	}
	if view.Type != "place" && view.Type != "event" && view.Type != "fix" {
		view.Type = "place"
	}
	if fixID != "" {
		view.Type = "fix"
		if item, err := p.loadItem(r.Context(), fixID); err == nil {
			view.FixItem = &item
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	p.render(w, r, templates.AddPage(view))
}

// GET /add/success — подтверждение отправки предложения.
func (p *Pages) AddSuccessPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	p.render(w, r, templates.AddSuccessPage(templates.AddSuccessView{
		User: p.viewUser(r),
		Type: r.URL.Query().Get("type"),
	}))
}

// GET /profile — метки, отметки «Я приду» и предложения пользователя.
func (p *Pages) ProfilePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	user := p.viewUser(r)
	if user == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		p.render(w, r, templates.ProfilePage(templates.ProfileView{Tab: parseProfileTab(r)}))

		return
	}

	view, err := p.loadProfileView(r.Context(), r, user, parseProfileTab(r))
	if err != nil {
		p.pageError(w, r, "profile page", err)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	p.render(w, r, templates.ProfilePage(view))
}

func parseProfileTab(r *http.Request) string {
	tab := r.URL.Query().Get("tab")
	if tab != "going" && tab != "submissions" {
		tab = "pins"
	}

	return tab
}

func (p *Pages) loadProfileView(ctx context.Context, r *http.Request, user *presenter.User, tab string) (templates.ProfileView, error) {
	pins, err := p.listPins.Do(ctx, models.PinFilter{Limit: 200, Viewer: &user.ID})
	if err != nil {
		return templates.ProfileView{}, err
	}

	submissions, err := p.listSubmissions.Do(ctx, models.SubmissionFilter{AuthorID: user.ID, Limit: 100})
	if err != nil {
		return templates.ProfileView{}, err
	}

	return templates.ProfileView{
		User:        user,
		Tab:         tab,
		Pins:        presenter.PinsToHTTP(pins),
		Submissions: presenter.SubmissionsToHTTP(submissions),
	}, nil
}

// GET /rules — правила меток и модерации.
func (p *Pages) RulesPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method GET expected", http.StatusMethodNotAllowed)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	p.render(w, r, templates.RulesPage(templates.RulesView{
		User:    p.viewUser(r),
		Section: r.URL.Query().Get("section"),
	}))
}

// NotFoundPage рендерит страницу 404 для несуществующих путей.
func (p *Pages) NotFoundPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	p.render(w, r, templates.NotFoundPage(p.viewUser(r)))
}

func (p *Pages) render(w http.ResponseWriter, r *http.Request, component templ.Component) {
	if err := component.Render(r.Context(), w); err != nil {
		p.logger.Error("render page failed", slog.Any("error", err))
	}
}

func (p *Pages) pageError(w http.ResponseWriter, r *http.Request, what string, err error) {
	p.logger.Error(what+" failed", slog.Any("error", err))
	http.Error(w, "internal error", statusCode(err))
}
