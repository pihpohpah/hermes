package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/pihpohpah/hermes/internal/application/presenter"
	"github.com/pihpohpah/hermes/internal/domain/models"
)

type ListPlacesUC interface {
	Do(ctx context.Context, filter models.PlaceFilter) ([]models.Place, error)
}

type ListPlacesHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[[]presenter.Place]
	useCase        ListPlacesUC
}

func NewListPlacesHandler(logger *slog.Logger, uc ListPlacesUC) *ListPlacesHandler {
	return &ListPlacesHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[[]presenter.Place](logger),
		useCase:        uc,
	}
}

func (h *ListPlacesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	filter, err := parsePlaceFilter(r)
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	places, err := h.useCase.Do(r.Context(), filter)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.PlacesToHTTP(places))
}

// parsePlaceFilter reads query parameters into a domain filter. Returning domain
// errors keeps the HTTP layer thin: statusCode maps them to 400.
func parsePlaceFilter(r *http.Request) (models.PlaceFilter, error) {
	filter := models.PlaceFilter{}
	query := r.URL.Query()

	if raw := query.Get("type"); raw != "" {
		parsed, err := models.NewPlaceType(raw)
		if err != nil {
			return filter, err
		}

		filter.Type = &parsed
	}

	if raw := query.Get("layer"); raw != "" {
		parsed, err := models.NewLayerCode(raw)
		if err != nil {
			return filter, err
		}

		filter.Layer = &parsed
	}

	if raw := query.Get("category"); raw != "" {
		parsed, err := models.NewCategoryCode(raw)
		if err != nil {
			return filter, err
		}

		filter.Category = &parsed
	}

	if raw := query.Get("q"); raw != "" {
		trimmed := strings.TrimSpace(raw)
		filter.Q = &trimmed
	}

	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return filter, models.Invalid("`limit` must be an integer")
		}

		filter.Limit = parsed
	}

	lat, lon := query.Get("lat"), query.Get("lon")
	radius := query.Get("radius_m")

	if lat != "" || lon != "" {
		if lat == "" || lon == "" {
			return filter, models.Invalid("both `lat` and `lon` are required for a radius search")
		}

		latValue, err := strconv.ParseFloat(lat, 64)
		if err != nil {
			return filter, models.Invalid("`lat` must be a number")
		}

		lonValue, err := strconv.ParseFloat(lon, 64)
		if err != nil {
			return filter, models.Invalid("`lon` must be a number")
		}

		center, err := models.NewPoint(latValue, lonValue)
		if err != nil {
			return filter, err
		}

		filter.Center = &center

		// Радиус обязателен вместе с центром: без него PostGIS-фильтр не строится.
		if radius == "" {
			return filter, models.Invalid("`radius_m` is required together with `lat`/`lon`")
		}

		radiusValue, err := strconv.ParseFloat(radius, 64)
		if err != nil {
			return filter, models.Invalid("`radius_m` must be a number")
		}

		parsedRadius, err := models.NewRadiusMeters(radiusValue)
		if err != nil {
			return filter, err
		}

		filter.RadiusM = &parsedRadius
	}

	return filter, nil
}

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

type CreatePlaceUC interface {
	Do(ctx context.Context, in models.CreatePlaceInput) (models.Place, error)
}

type CreatePlaceHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.Place]
	useCase        CreatePlaceUC
}

func NewCreatePlaceHandler(logger *slog.Logger, uc CreatePlaceUC) *CreatePlaceHandler {
	return &CreatePlaceHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.Place](logger),
		useCase:        uc,
	}
}

type createPlaceRequest struct {
	Name            string         `json:"name"`
	Slug            *string        `json:"slug"`
	Subtitle        string         `json:"subtitle"`
	Short           string         `json:"short"`
	Description     string         `json:"description"`
	Type            string         `json:"type"`
	Layer           string         `json:"layer"`
	Category        *string        `json:"category"`
	District        string         `json:"district"`
	Address         string         `json:"address"`
	Lat             float64        `json:"lat"`
	Lon             float64        `json:"lon"`
	Hours           string         `json:"hours"`
	PriceFree       *bool          `json:"price_free"`
	PriceLabel      string         `json:"price_label"`
	Duration        string         `json:"duration"`
	Season          string         `json:"season"`
	Access          []string       `json:"access"`
	HowToGet        string         `json:"how_to_get"`
	Facts           []string       `json:"facts"`
	Tags            []string       `json:"tags"`
	Photos          []models.Photo `json:"photos"`
	Beach           *string        `json:"beach"`
	SwimSeason      string         `json:"swim_season"`
	Elevation       *int           `json:"elevation"`
	ElevationSource string         `json:"elevation_source"`
	Difficulty      *int           `json:"difficulty"`
	OSMID           *int64         `json:"osm_id"`
}

func (h *CreatePlaceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method POST expected"))

		return
	}

	var body createPlaceRequest
	if err := decodeJSON(r, &body); err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	input, err := body.toDomain()
	if err != nil {
		h.responseWriter.Error(w, http.StatusBadRequest, err)

		return
	}

	place, err := h.useCase.Do(r.Context(), input)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.SuccessWithStatus(w, http.StatusCreated, presenter.PlaceToHTTP(place))
}

func (b createPlaceRequest) toDomain() (models.CreatePlaceInput, error) {
	placeType, err := models.NewPlaceType(b.Type)
	if err != nil {
		return models.CreatePlaceInput{}, err
	}

	layer, err := models.NewLayerCode(b.Layer)
	if err != nil {
		return models.CreatePlaceInput{}, err
	}

	location, err := models.NewPoint(b.Lat, b.Lon)
	if err != nil {
		return models.CreatePlaceInput{}, err
	}

	if b.Name == "" {
		return models.CreatePlaceInput{}, models.Invalid("`name` is required")
	}

	elevationSource, err := models.NewElevationSource(b.ElevationSource)
	if err != nil {
		return models.CreatePlaceInput{}, err
	}

	var difficulty *models.Difficulty
	if b.Difficulty != nil {
		parsed, diffErr := models.NewDifficulty(*b.Difficulty)
		if diffErr != nil {
			return models.CreatePlaceInput{}, diffErr
		}

		difficulty = &parsed
	}

	var category *models.CategoryCode
	if b.Category != nil && strings.TrimSpace(*b.Category) != "" {
		parsed, catErr := models.NewCategoryCode(*b.Category)
		if catErr != nil {
			return models.CreatePlaceInput{}, catErr
		}
		category = &parsed
	}

	access := make([]models.AccessMode, 0, len(b.Access))
	for _, raw := range b.Access {
		mode, modeErr := models.NewAccessMode(raw)
		if modeErr != nil {
			return models.CreatePlaceInput{}, modeErr
		}
		access = append(access, mode)
	}

	var beach *models.BeachType
	if b.Beach != nil && strings.TrimSpace(*b.Beach) != "" {
		parsed, beachErr := models.NewBeachType(*b.Beach)
		if beachErr != nil {
			return models.CreatePlaceInput{}, beachErr
		}
		beach = &parsed
	}

	return models.CreatePlaceInput{
		Name:            b.Name,
		Slug:            b.Slug,
		Subtitle:        b.Subtitle,
		Short:           b.Short,
		Description:     b.Description,
		Type:            placeType,
		Layer:           layer,
		Category:        category,
		District:        b.District,
		Address:         b.Address,
		Location:        location,
		Hours:           b.Hours,
		PriceFree:       b.PriceFree,
		PriceLabel:      b.PriceLabel,
		Duration:        b.Duration,
		Season:          b.Season,
		Access:          access,
		HowToGet:        b.HowToGet,
		Facts:           b.Facts,
		Tags:            b.Tags,
		Photos:          b.Photos,
		Beach:           beach,
		SwimSeason:      b.SwimSeason,
		Elevation:       b.Elevation,
		ElevationSource: elevationSource,
		Difficulty:      difficulty,
		OSMID:           b.OSMID,
	}, nil
}

type ListSimilarPlacesUC interface {
	Do(ctx context.Context, id string, limit int) ([]models.Place, error)
}

type ListSimilarPlacesHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[[]presenter.Place]
	useCase        ListSimilarPlacesUC
}

func NewListSimilarPlacesHandler(logger *slog.Logger, uc ListSimilarPlacesUC) *ListSimilarPlacesHandler {
	return &ListSimilarPlacesHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[[]presenter.Place](logger),
		useCase:        uc,
	}
}

// GET /places/{id}/similar?limit=6
func (h *ListSimilarPlacesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			h.responseWriter.Error(w, http.StatusBadRequest, models.Invalid("`limit` must be a positive integer"))

			return
		}
		limit = parsed
	}

	places, err := h.useCase.Do(r.Context(), r.PathValue("id"), limit)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.PlacesToHTTP(places))
}

type PlaceStatsUC interface {
	Do(ctx context.Context, layer *models.LayerCode) (models.PlaceStats, error)
}

type PlaceStatsHandler struct {
	logger         *slog.Logger
	responseWriter *APIResponseWriter[presenter.PlaceStats]
	useCase        PlaceStatsUC
}

func NewPlaceStatsHandler(logger *slog.Logger, uc PlaceStatsUC) *PlaceStatsHandler {
	return &PlaceStatsHandler{
		logger:         logger,
		responseWriter: NewAPIResponseWriter[presenter.PlaceStats](logger),
		useCase:        uc,
	}
}

// GET /places/stats?scenario=sea — catalog aggregates for the directory
// tabs: entry counts by category, plus the place/event split.
func (h *PlaceStatsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responseWriter.Error(w, http.StatusMethodNotAllowed, errors.New("method GET expected"))

		return
	}

	var scenario *models.LayerCode
	if raw := r.URL.Query().Get("scenario"); raw != "" {
		layer, err := models.NewLayerCode(raw)
		if err != nil {
			h.responseWriter.Error(w, http.StatusBadRequest, err)

			return
		}
		scenario = &layer
	}

	stats, err := h.useCase.Do(r.Context(), scenario)
	if err != nil {
		h.responseWriter.Error(w, statusCode(err), err)

		return
	}

	h.responseWriter.Success(w, presenter.PlaceStatsToHTTP(stats))
}
