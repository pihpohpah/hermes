package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

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
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	Type            string  `json:"type"`
	Layer           string  `json:"layer"`
	Lat             float64 `json:"lat"`
	Lon             float64 `json:"lon"`
	Elevation       *int    `json:"elevation"`
	ElevationSource string  `json:"elevation_source"`
	Difficulty      *int    `json:"difficulty"`
	OSMID           *int64  `json:"osm_id"`
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

	return models.CreatePlaceInput{
		Name:            b.Name,
		Description:     b.Description,
		Type:            placeType,
		Layer:           layer,
		Location:        location,
		Elevation:       b.Elevation,
		ElevationSource: elevationSource,
		Difficulty:      difficulty,
		OSMID:           b.OSMID,
	}, nil
}
