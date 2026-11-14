package routes

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof" // trigger init
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
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
	ListSimilarPlaces  *httproutes.ListSimilarPlacesHandler
	PlaceStats         *httproutes.PlaceStatsHandler
	ListEvents         *httproutes.ListEventsHandler
	GetEvent           *httproutes.GetEventHandler
	ListCollections    *httproutes.ListCollectionsHandler
	GetCollection      *httproutes.GetCollectionHandler
	ListPins           *httproutes.ListPinsHandler
	GetPin             *httproutes.GetPinHandler
	CreatePin          *httproutes.CreatePinHandler
	DeletePin          *httproutes.DeletePinHandler
	SetPinGoing        *httproutes.SetPinGoingHandler
	ReportPin          *httproutes.ReportPinHandler
	ListTrips          *httproutes.ListTripsHandler
	CreateTrip         *httproutes.CreateTripHandler
	UpdateTrip         *httproutes.UpdateTripHandler
	ListReports        *httproutes.ListReportsHandler
	CreateReport       *httproutes.CreateReportHandler
	CreateUser         *httproutes.CreateUserHandler
	GetCurrentUser     *httproutes.GetCurrentUserHandler
	CreateSubmission   *httproutes.CreateSubmissionHandler
	GetSubmission      *httproutes.GetSubmissionHandler
	ListSubmissions    *httproutes.ListSubmissionsHandler
	PhotoUpload        *httproutes.PhotoUploadHandler
	RequireAuth        func(http.Handler) http.Handler
	LoadAuth           func(http.Handler) http.Handler
	AllowedOrigins     []string
	Pages              *httproutes.Pages
	Fragments          *httproutes.Fragments
	MapItems           *httproutes.MapItems
	Item               *httproutes.Item
	Logout             *httproutes.LogoutHandler
	UpdateUser         *httproutes.UpdateCurrentUserHandler
	TelegramLogin      *httproutes.TelegramLoginHandler
	TelegramWidget     *httproutes.TelegramWidgetConfigHandler
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

	mux.Handle("POST /users", h.CreateUser)

	requireAuth := h.RequireAuth
	if requireAuth == nil {
		requireAuth = func(next http.Handler) http.Handler { return next }
	}
	loadAuth := h.LoadAuth
	if loadAuth == nil {
		loadAuth = func(next http.Handler) http.Handler { return next }
	}

	mux.Handle("GET /users/me", requireAuth(h.GetCurrentUser))
	if h.UpdateUser != nil {
		mux.Handle("PATCH /users/me", requireAuth(h.UpdateUser))
	}
	if h.Logout != nil {
		mux.Handle("POST /users/logout", requireAuth(h.Logout))
	}
	if h.TelegramLogin != nil {
		mux.Handle("POST /auth/telegram", h.TelegramLogin)
	}
	if h.TelegramWidget != nil {
		mux.Handle("GET /auth/telegram/widget", h.TelegramWidget)
	}

	mux.Handle("GET /places", h.ListPlaces)
	mux.Handle("POST /places", h.CreatePlace)
	mux.Handle("GET /places/stats", h.PlaceStats)
	mux.Handle("GET /places/{id}", h.GetPlace)
	mux.Handle("GET /places/{id}/similar", h.ListSimilarPlaces)

	mux.Handle("GET /events", h.ListEvents)
	mux.Handle("GET /events/{id}", h.GetEvent)

	mux.Handle("GET /collections", h.ListCollections)
	mux.Handle("GET /collections/{id}", h.GetCollection)

	// Радар «Сейчас»: чтение доступно анонимно (с декорированием для своих),
	// мутации — только после входа.
	mux.Handle("GET /pins", loadAuth(h.ListPins))
	mux.Handle("GET /pins/{id}", loadAuth(h.GetPin))
	mux.Handle("POST /pins", requireAuth(h.CreatePin))
	mux.Handle("DELETE /pins/{id}", requireAuth(h.DeletePin))
	mux.Handle("PUT /pins/{id}/going", requireAuth(h.SetPinGoing))
	mux.Handle("DELETE /pins/{id}/going", requireAuth(h.SetPinGoing))
	mux.Handle("POST /pins/{id}/reports", requireAuth(h.ReportPin))

	// Предложения пользователей (места, события, правки) и загрузка фото.
	mux.Handle("POST /submissions", requireAuth(h.CreateSubmission))
	mux.Handle("GET /submissions", requireAuth(h.ListSubmissions))
	mux.Handle("GET /submissions/{id}", requireAuth(h.GetSubmission))
	if h.PhotoUpload != nil {
		mux.Handle("POST /photos", requireAuth(h.PhotoUpload))
	}

	mux.Handle("GET /forecast/place/{id}", h.GetPlaceForecast)
	mux.Handle("POST /forecast/refresh", h.RefreshForecast)

	// Trips and report creation are personal: the owner comes from the
	// session, so every mutation (and the personal listing) requires auth.
	mux.Handle("GET /trips", requireAuth(h.ListTrips))
	mux.Handle("POST /trips", requireAuth(h.CreateTrip))
	mux.Handle("PATCH /trips/{id}", requireAuth(h.UpdateTrip))

	if h.ListReports != nil {
		mux.Handle("GET /reports", h.ListReports)
	}
	if h.CreateReport != nil {
		mux.Handle("POST /reports", requireAuth(h.CreateReport))
	}

	// Серверно-рендеренные страницы продукта и htmx-фрагменты.
	if h.Pages != nil {
		mux.Handle("GET /{$}", loadAuth(http.HandlerFunc(h.Pages.MapPage)))
		mux.Handle("GET /now", loadAuth(http.HandlerFunc(h.Pages.NowPage)))
		mux.Handle("GET /catalog", loadAuth(http.HandlerFunc(h.Pages.CatalogPage)))
		mux.Handle("GET /add", loadAuth(http.HandlerFunc(h.Pages.AddPage)))
		mux.Handle("GET /add/success", loadAuth(http.HandlerFunc(h.Pages.AddSuccessPage)))
		mux.Handle("GET /profile", loadAuth(http.HandlerFunc(h.Pages.ProfilePage)))
		mux.Handle("GET /rules", loadAuth(http.HandlerFunc(h.Pages.RulesPage)))
		mux.Handle("GET /place/{id}", loadAuth(http.HandlerFunc(h.Pages.PlacePage)))
		mux.Handle("GET /event/{id}", loadAuth(http.HandlerFunc(h.Pages.EventPage)))
	}
	if h.Fragments != nil {
		mux.Handle("GET /fragments/map-panel", loadAuth(http.HandlerFunc(h.Fragments.MapPanel)))
		mux.Handle("GET /fragments/catalog-results", loadAuth(http.HandlerFunc(h.Fragments.CatalogResults)))
		mux.Handle("GET /fragments/pins", loadAuth(http.HandlerFunc(h.Fragments.Pins)))
		mux.Handle("GET /fragments/pin/{id}", loadAuth(http.HandlerFunc(h.Fragments.Pin)))
		mux.Handle("GET /fragments/profile-tab", loadAuth(http.HandlerFunc(h.Fragments.ProfileTab)))
		mux.Handle("GET /fragments/live-count", http.HandlerFunc(h.Fragments.LiveCount))
	}
	if h.MapItems != nil {
		mux.Handle("GET /api/map-items", loadAuth(h.MapItems))
	}
	if h.Item != nil {
		mux.Handle("GET /api/items/{id}", loadAuth(h.Item))
	}

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

	if h.WebDir != "" {
		files := http.FileServer(http.Dir(h.WebDir))
		mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Существующие файлы отдаём как есть; остальное — страница 404 продукта.
			if info, err := os.Stat(filepath.Join(h.WebDir, filepath.Clean(r.URL.Path))); err != nil || info.IsDir() {
				if h.Pages != nil {
					h.Pages.NotFoundPage(w, r)

					return
				}
				http.NotFound(w, r)

				return
			}
			files.ServeHTTP(w, r)
		}))
	}

	return withRecover(logger, withCORS(h.AllowedOrigins, withRequestLog(logger, mux)))
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

// withCORS reflects the request origin only when it is on the trusted list,
// so credentialed cross-origin access works for known frontends and is denied
// everywhere else. An empty trusted list means "same-origin only": no CORS
// headers are emitted at all, which is the safe default for local dev.
func withCORS(allowed []string, next http.Handler) http.Handler {
	trusted := make(map[string]struct{}, len(allowed))
	for _, origin := range allowed {
		if origin = strings.TrimSpace(origin); origin != "" {
			trusted[origin] = struct{}{}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		_, ok := trusted[origin]
		if origin != "" && ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers",
				"Accept, Content-Type, HX-Request, HX-Target, HX-Trigger, Range, Idempotency-Key, X-CSRF-Token")
			w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges, HX-Trigger")
			w.Header().Add("Vary", "Origin")
		}

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
