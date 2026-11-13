package routes

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof" // trigger init
	"runtime/debug"
	"time"

	httproutes "github.com/pihpohpah/hermes/internal/application/routes/http"
)

type Handlers struct {
	Health             *httproutes.HealthCheckHandler
	ListPlaces         *httproutes.ListPlacesHandler
	GetPlace           *httproutes.GetPlaceHandler
	CreatePlace        *httproutes.CreatePlaceHandler
	GetPlaceForecast   *httproutes.GetPlaceForecastHandler
	RefreshForecast    *httproutes.RefreshForecastHandler
	ListTrips          *httproutes.ListTripsHandler
	CreateTrip         *httproutes.CreateTripHandler
	UpdateTrip         *httproutes.UpdateTripHandler
	ListReports        *httproutes.ListReportsHandler
	CreateReport       *httproutes.CreateReportHandler
	PlacesListFragment *httproutes.PlacesListFragmentHandler
	PlaceCardFragment  *httproutes.PlaceCardFragmentHandler
	IndexPage          *httproutes.IndexPageHandler
	MapPage            *httproutes.MapPageHandler
	MapAssets          *httproutes.MapAssetsHandler
	PMTiles            *httproutes.PMTilesHandler
	Relief             *httproutes.PMTilesHandler
	WebDir             string
}

func NewHTTPRouter(logger *slog.Logger, h Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /health", h.Health)
	mux.Handle("HEAD /health", h.Health)

	mux.Handle("GET /places", h.ListPlaces)
	mux.Handle("POST /places", h.CreatePlace)
	mux.Handle("GET /places/{id}", h.GetPlace)

	mux.Handle("GET /forecast/place/{id}", h.GetPlaceForecast)
	mux.Handle("POST /forecast/refresh", h.RefreshForecast)

	mux.Handle("GET /trips", h.ListTrips)
	mux.Handle("POST /trips", h.CreateTrip)
	mux.Handle("PATCH /trips/{id}", h.UpdateTrip)

	if h.ListReports != nil {
		mux.Handle("GET /reports", h.ListReports)
	}
	if h.CreateReport != nil {
		mux.Handle("POST /reports", h.CreateReport)
	}

	mux.Handle("GET /fragments/places-list", h.PlacesListFragment)
	mux.Handle("GET /fragments/place-card/{id}", h.PlaceCardFragment)

	mux.Handle("GET /api/pmtiles/region.pmtiles", h.PMTiles)
	mux.Handle("HEAD /api/pmtiles/region.pmtiles", h.PMTiles)

	// Офлайн-карта: страница /map и её локальные ассеты (MapLibre, PMTiles, глифы).
	if h.MapPage != nil {
		mux.Handle("GET /map/{$}", h.MapPage)
	}
	if h.MapAssets != nil {
		mux.Handle("GET /map/", h.MapAssets)
	}

	// Архив карты по адресу из задания. Отдаётся байтовыми диапазонами.
	mux.Handle("GET /api/map/region.pmtiles", h.PMTiles)
	mux.Handle("HEAD /api/map/region.pmtiles", h.PMTiles)

	// Рельеф отдельным растровым архивом: векторные тайлы высот не содержат.
	if h.Relief != nil {
		mux.Handle("GET /api/map/relief.pmtiles", h.Relief)
		mux.Handle("HEAD /api/map/relief.pmtiles", h.Relief)
	}

	if h.IndexPage != nil {
		mux.Handle("GET /{$}", h.IndexPage)
	}
	if h.WebDir != "" {
		mux.Handle("GET /", http.FileServer(http.Dir(h.WebDir)))
	}

	return withRecover(logger, withCORS(withRequestLog(logger, mux)))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func withRequestLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}
		next.ServeHTTP(rec, r)
		logger.Info(
			"http request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, HX-Request, HX-Target, HX-Trigger, Range")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges, HX-Trigger")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)

			return
		}

		next.ServeHTTP(w, r)
	})
}

func withRecover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error(
					"panic recovered",
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write(
					[]byte(`{"success":false,"error":"internal server error","code":"INTERNAL_ERROR","data":null}`),
				)
			}
		}()

		next.ServeHTTP(w, r)
	})
}
