# Устройство `usecase`, `presenter` и `routes`

## 1. Usecase с интерфейсом на стороне потребителя (`internal/application/usecase`)

Каждый сценарий — отдельная структура с методом `Do(ctx, ...)` и конструктором `New<Name>UseCase`. Зависимость на
хранилище описывает неэкспортируемый интерфейс в том же файле, и в нём только методы, нужные этому сценарию
(`placeLister`, `placeGetter`, `placeCreator`). Один репозиторий закрывает несколько таких интерфейсов.

Usecase проверяет бизнес-правила и возвращает доменные ошибки (`models.Invalid(...)`, `models.ErrNotFound`), а не
HTTP-коды.

### Пример `internal/application/usecase/places.go`

```go
// GetPlaceUseCase loads a single place by id.
type GetPlaceUseCase struct {
	places placeGetter
}

type placeGetter interface {
	GetByID(ctx context.Context, id string) (models.Place, error)
}

func NewGetPlaceUseCase(places placeGetter) *GetPlaceUseCase {
	return &GetPlaceUseCase{places: places}
}

func (u *GetPlaceUseCase) Do(ctx context.Context, id string) (models.Place, error) {
	if id == "" {
		return models.Place{}, models.Invalid("`id` is required")
	}

	// id may be a UUID or the human-readable slug from the URL.
	return u.places.GetByID(ctx, id)
}

// ListPlacesUseCase returns places filtered by layer, type or radius around a
// point. An empty filter lists the catalogue page by page.
type ListPlacesUseCase struct {
	places placeLister
}

type placeLister interface {
	List(ctx context.Context, filter models.PlaceFilter) ([]models.Place, error)
}

func NewListPlacesUseCase(places placeLister) *ListPlacesUseCase {
	return &ListPlacesUseCase{places: places}
}

func (u *ListPlacesUseCase) Do(ctx context.Context, filter models.PlaceFilter) ([]models.Place, error) {
	if filter.Limit <= 0 {
		filter.Limit = defaultPlaceListLimit
	}

	return u.places.List(ctx, filter)
}
```

## 2. Presenter: доменная модель → DTO (`internal/application/presenter`)

JSON-форму ответа задаёт `presenter`: DTO с тегами `json` и функции `<Model>ToHTTP`. Доменные сущности тегов не несут.
Здесь же переименовываются поля (`Location` → `lat`/`lon`), слайсы value objects превращаются в строки, а `nil`
остаётся `null`, а не нулём.

### Пример `internal/application/presenter/places.go` (фрагмент)

```go
type Place struct {
	ID          string           `json:"id"`
	Slug        string           `json:"slug,omitempty"`
	Name        string           `json:"name"`
	Type        models.PlaceType `json:"type"`
	Layer       models.LayerCode `json:"layer"`
	Lat         float64          `json:"lat"`
	Lon         float64          `json:"lon"`
	PriceFree   *bool            `json:"price_free,omitempty"`
	Access      []string         `json:"access"`
	// ...
}

func PlaceToHTTP(in models.Place) Place {
	access := make([]string, 0, len(in.Access))
	for _, mode := range in.Access {
		access = append(access, string(mode))
	}

	out := Place{
		ID:        in.ID,
		Name:      in.Name,
		Type:      in.Type,
		Layer:     in.Layer,
		Lat:       in.Location.Lat,
		Lon:       in.Location.Lon,
		PriceFree: in.PriceFree,
		Access:    access,
		// ...
	}
	// ...

	return out
}

func PlacesToHTTP(in []models.Place) []Place {
	out := make([]Place, len(in))
	for i := range in {
		out[i] = PlaceToHTTP(in[i])
	}

	return out
}
```

## 3. HTTP-роутер (`internal/application/routes/router.go`)

Роутер построен на `http.NewServeMux()` с шаблонами Go 1.22+: метод в шаблоне (`"GET /places/{id}"`), параметр пути
читается через `r.PathValue("id")`. Хендлеры приходят одной структурой `routes.Handlers` из `cmd/server/listen.go`.

В роутере регистрируются:

- служебные ручки: `GET /health`, `HEAD /health` и `net/http/pprof` через blank import;
- JSON API: `/places`, `/events`, `/pins`, `/forecast/...`, `/submissions`;
- страницы на `templ` (`GET /catalog`, `GET /place/{id}`) и htmx-фрагменты (`GET /fragments/...`);
- архивы карты (`/api/map/*.pmtiles`) и статика из `WebDir`.

Доступ задают middleware: `requireAuth` для мутаций, `loadAuth` для чтения, которое по-разному показывается
анонимам и вошедшим. Снаружи весь mux оборачивают `withRecover`, `withCORS` и `withRequestLog`.

### Сокращённый `router.go`

```go
func NewHTTPRouter(logger *slog.Logger, h Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /health", h.Health)
	mux.Handle("HEAD /health", h.Health)

	mux.Handle("GET /places", h.ListPlaces)
	mux.Handle("POST /places", h.CreatePlace)
	mux.Handle("GET /places/{id}", h.GetPlace)

	// Радар «Сейчас»: чтение доступно анонимно (с декорированием для своих),
	// мутации — только после входа.
	mux.Handle("GET /pins", loadAuth(h.ListPins))
	mux.Handle("POST /pins", requireAuth(h.CreatePin))

	if h.Fragments != nil {
		mux.Handle("GET /fragments/catalog-results", loadAuth(http.HandlerFunc(h.Fragments.CatalogResults)))
	}
	// ...

	return withRecover(logger, withCORS(h.AllowedOrigins, withRequestLog(logger, mux)))
}
```

## 4. Единый JSON-ответ и хендлеры (`internal/application/routes/http`)

### 4.1. Обёртка ответа (`common.go`)

Все JSON-ручки отвечают в одном формате, чтобы фронтенд ветвился по `success`, не разбирая статус:

```json
{
  "success": false,
  "error": "unexpected activity `ski`, expected: hike, sup, run, walk",
  "code": "INVALID_ARGUMENT",
  "data": null
}
```

`code` появляется только в ошибках. Ответ пишет дженерик `APIResponseWriter[T]`:

- `Success(w, data)` отвечает 200;
- `SuccessWithStatus(w, code, data)` нужен для 201 и похожих;
- `Error(w, status, err)` пишет ошибку в лог и в ответ.

Доменную ошибку в HTTP-статус переводит `statusCode(err)`: `ErrInvalidArgument` → 400, `ErrNotFound` → 404,
`ErrUnauthorized` → 401, `ErrForbidden` → 403, `ErrConflict` → 409, `context.Canceled` → 499,
`context.DeadlineExceeded` → 504, остальное → 500. Тело запроса читает `decodeJSON`: он ограничивает размер
(`maxJSONBodyBytes`) и отклоняет неизвестные поля.

```go
// APIResponse is the single envelope every JSON endpoint returns, so the front
// end can branch on `success` without inspecting status codes.
type APIResponse[data any] struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Data    data   `json:"data"`
}

type APIResponseWriter[data any] struct {
	logger *slog.Logger
}

func NewAPIResponseWriter[data any](logger *slog.Logger) *APIResponseWriter[data] {
	return &APIResponseWriter[data]{
		logger: logger,
	}
}

func (a *APIResponseWriter[data]) Error(writer http.ResponseWriter, code int, err error) {
	a.logger.Error("request failed", slog.Any("error", err), slog.Int("status", code))

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(code)

	if encodeErr := json.NewEncoder(writer).Encode(APIResponse[any]{
		Success: false,
		Error:   err.Error(),
		Code:    errorCode(code, err),
		Data:    nil,
	}); encodeErr != nil {
		a.logger.Error("json encode of error response failed", slog.Any("error", encodeErr))
	}
}

func (a *APIResponseWriter[data]) Success(writer http.ResponseWriter, d data) {
	a.SuccessWithStatus(writer, http.StatusOK, d)
}
```

### 4.2. Хендлер (`places.go`) и health check (`health_check.go`)

Хендлер:

1. Хранит типизированный `*APIResponseWriter[<DTO>]` и экспортируемый интерфейс usecase (`GetPlaceUC`), объявленный
   рядом с ним.
2. Реализует `ServeHTTP` и сам проверяет метод: роутер уже фильтрует по методу, а проверка защищает хендлер, если
   его подключат иначе.
3. Разбирает параметры запроса доменными конструкторами (`models.NewActivity`, `models.NewLayerCode`). Ошибка
   конструктора уже несёт `ErrInvalidArgument`, поэтому `statusCode(err)` вернёт 400.
4. Вызывает `useCase.Do(r.Context(), ...)` и переводит ошибку через `statusCode(err)`.
5. Отдаёт результат через `presenter` и `responseWriter.Success`.

```go
type GetPlaceUC interface {
	Do(ctx context.Context, id string) (models.Place, error)
}

type GetPlaceHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.Place]
	useCase        GetPlaceUC
}

func NewGetPlaceHandler(logger *slog.Logger, uc GetPlaceUC) *GetPlaceHandler {
	return &GetPlaceHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.Place](logger),
		useCase:        uc,
	}
}

func (h *GetPlaceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	place, err := h.useCase.Do(r.Context(), r.PathValue("id"))
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.PlaceToHTTP(place))
}
```

Health check зависит от узкого `dbPinger`, а не от `*pgxpool.Pool`, и пингует базу с собственным таймаутом:

```go
// HealthCheckHandler reports whether the service can reach PostgreSQL.
type HealthCheckHandler struct {
	logger *slog.Logger
	pinger dbPinger
	writer *APIResponseWriter[healthResponse]
}

type dbPinger interface {
	Ping(ctx context.Context) error
}

func (h *HealthCheckHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.writer.Error(w, http.StatusMethodNotAllowed, errors.New("method GET or HEAD expected"))

		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
	defer cancel()

	if err := h.pinger.Ping(ctx); err != nil {
		h.logger.Error("health check: database unreachable", slog.Any("error", err))
		h.writer.Error(w, http.StatusServiceUnavailable, errors.New("database unreachable"))

		return
	}

	h.writer.Success(w, healthResponse{Status: "ok", Database: "ok"})
}
```

### 4.3. Страницы и фрагменты

Страницы (`pages.go`) и htmx-фрагменты (`fragments.go`) рендерят HTML-шаблоны `templ` из `web/templates/`, а не JSON.
Они берут те же usecase-ы, что и JSON-ручки. `ErrNotFound` на странице показывает `NotFoundPage`, а не JSON-ошибку.
Правила фронтенда описаны в скилле `frontend-hypermedia-architecture`.

Если появится gRPC, его сервер берёт те же usecase-ы, переводит proto-enum доменными конструкторами и маппит
`ErrInvalidArgument` в `codes.InvalidArgument`.
