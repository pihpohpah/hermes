package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/application/routes"
	httproutes "github.com/pihpohpah/hermes/internal/application/routes/http"
	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/domain/models"
	"github.com/pihpohpah/hermes/internal/infrastructure/config"
	"github.com/pihpohpah/hermes/internal/infrastructure/openmeteo"
	"github.com/pihpohpah/hermes/internal/infrastructure/photostore"
	"github.com/pihpohpah/hermes/internal/infrastructure/repository"
)

const idleTimeout = time.Minute

// photoStoreFromConfig resolves the photo storage root and the public URL
// prefix. By default photos go to <WebDir>/uploads and are served at
// /uploads/... by the static handler. An explicit PHOTOS_STORAGE_DIR wins;
// when it points outside the web dir the prefix comes back empty and the
// caller logs a warning.
func photoStoreFromConfig(cfg config.Config) (*photostore.Store, string) {
	dir := cfg.PhotosConfig.StorageDir
	if dir == "" {
		dir = filepath.Join(cfg.WebDir, "uploads")
	}

	prefix := ""
	if rel, err := filepath.Rel(cfg.WebDir, dir); err == nil && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && filepath.IsLocal(rel) {
		prefix = "/" + filepath.ToSlash(rel)
	}

	return &photostore.Store{Dir: dir, Prefix: prefix}, prefix
}

// pinRulesFromConfig maps the PINS_* deployment settings onto the radar
// moderation policy. The night window is evaluated in Asia/Vladivostok.
func pinRulesFromConfig(cfg config.Config) usecase.PinRules {
	location, err := models.PrimoryeLocation()
	if err != nil {
		location = time.UTC
	}

	return usecase.PinRules{
		MaxActivePerUser:    cfg.PinsConfig.MaxActivePerUser,
		MaxPerHour:          cfg.PinsConfig.MaxPerHour,
		StopWords:           cfg.PinsConfig.StopWordList(),
		NightBanFrom:        cfg.PinsConfig.NightBanFrom,
		NightBanTo:          cfg.PinsConfig.NightBanTo,
		ReportHideThreshold: cfg.PinsConfig.ReportHideThreshold,
		MaxDuration:         time.Duration(cfg.PinsConfig.MaxDurationH * float64(time.Hour)),
		Location:            location,
	}
}

func runListen(
	ctx context.Context,
	logger *slog.Logger,
	cfg config.Config,
	pool *pgxpool.Pool,
	placeRepo *repository.PlaceRepository,
	weatherRepo *repository.WeatherRepository,
	forecastRepo *repository.ForecastRepository,
	tripRepo *repository.TripRepository,
	reportRepo *repository.ReportRepository,
	provider openmeteo.HourlyProvider,
) error {
	listPlacesUC := usecase.NewListPlacesUseCase(placeRepo)
	getPlaceUC := usecase.NewGetPlaceUseCase(placeRepo)
	createPlaceUC := usecase.NewCreatePlaceUseCase(placeRepo)
	listSimilarUC := usecase.NewListSimilarPlacesUseCase(placeRepo)
	placeStatsUC := usecase.NewPlaceStatsUseCase(placeRepo)

	eventRepo := repository.NewEventRepository(pool)
	collectionRepo := repository.NewCollectionRepository(pool)
	listEventsUC := usecase.NewListEventsUseCase(eventRepo, time.Now)
	getEventUC := usecase.NewGetEventUseCase(eventRepo)
	listCollectionsUC := usecase.NewListCollectionsUseCase(collectionRepo)
	getCollectionUC := usecase.NewGetCollectionUseCase(collectionRepo)

	pinRepo := repository.NewPinRepository(pool)
	pinRules := pinRulesFromConfig(cfg)
	listPinsUC := usecase.NewListPinsUseCase(pinRepo, time.Now)
	getPinUC := usecase.NewGetPinUseCase(pinRepo, time.Now)
	createPinUC := usecase.NewCreatePinUseCase(pinRepo, pinRules, time.Now)
	deletePinUC := usecase.NewDeletePinUseCase(pinRepo)
	setPinGoingUC := usecase.NewSetPinGoingUseCase(pinRepo, time.Now)
	reportPinUC := usecase.NewReportPinUseCase(pinRepo, pinRules, time.Now)

	getForecastUC := usecase.NewGetPlaceForecastUseCase(placeRepo, forecastRepo, weatherRepo, provider)
	refreshForecastUC := usecase.NewRefreshForecastUseCase(placeRepo, forecastRepo, weatherRepo, provider)

	listTripsUC := usecase.NewListTripsUseCase(tripRepo)
	createTripUC := usecase.NewCreateTripUseCase(tripRepo)
	updateTripUC := usecase.NewUpdateTripUseCase(tripRepo)

	listReportsUC := usecase.NewListReportsUseCase(reportRepo)
	createReportUC := usecase.NewCreateReportUseCase(reportRepo)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	idempotencyRepo := repository.NewIdempotencyRepository(pool)
	txManager := repository.NewTxManager(pool)

	photoRepo := repository.NewPhotoRepository(pool)
	createPhotoUC := usecase.NewCreatePhotoUseCase(photoRepo)
	photoStore, photoPrefix := photoStoreFromConfig(cfg)
	if photoPrefix == "" {
		logger.Warn("photos storage dir is outside the web dir, uploads will not be served; set PHOTOS_STORAGE_DIR inside WebDir")
	}

	createUserUC := usecase.NewCreateUserUseCase(userRepo, sessionRepo, idempotencyRepo, txManager, cfg.TTL)
	authMiddleware := httproutes.NewAuthMiddleware(logger, sessionRepo, sessionRepo, userRepo)

	submissionRepo := repository.NewSubmissionRepository(pool)
	createSubmissionUC := usecase.NewCreateSubmissionUseCase(
		submissionRepo,
		submissionRepo,
		idempotencyRepo,
		txManager,
		cfg.TTL,
	)
	getSubmissionUC := usecase.NewGetSubmissionUseCase(submissionRepo)
	listSubmissionsUC := usecase.NewListSubmissionsUseCase(submissionRepo)

	loader := httproutes.CatalogLoader(listPlacesUC, listEventsUC, getCollectionUC, placeRepo)
	pages := httproutes.NewPages(
		logger,
		loader,
		getPlaceUC,
		getEventUC,
		listSimilarUC,
		listEventsUC,
		listPinsUC,
		getPinUC,
		listCollectionsUC,
		listSubmissionsUC,
	)

	router := routes.NewHTTPRouter(logger, routes.Handlers{
		Health:             httproutes.NewHealthCheckHandler(logger, pool),
		ListPlaces:         httproutes.NewListPlacesHandler(logger, listPlacesUC),
		GetPlace:           httproutes.NewGetPlaceHandler(logger, getPlaceUC),
		CreatePlace:        httproutes.NewCreatePlaceHandler(logger, createPlaceUC),
		GetPlaceForecast:   httproutes.NewGetPlaceForecastHandler(logger, getForecastUC),
		RefreshForecast:    httproutes.NewRefreshForecastHandler(logger, refreshForecastUC),
		ListSimilarPlaces:  httproutes.NewListSimilarPlacesHandler(logger, listSimilarUC),
		PlaceStats:         httproutes.NewPlaceStatsHandler(logger, placeStatsUC),
		ListEvents:         httproutes.NewListEventsHandler(logger, listEventsUC),
		GetEvent:           httproutes.NewGetEventHandler(logger, getEventUC),
		ListCollections:    httproutes.NewListCollectionsHandler(logger, listCollectionsUC),
		GetCollection:      httproutes.NewGetCollectionHandler(logger, getCollectionUC, placeRepo, eventRepo),
		ListPins:           httproutes.NewListPinsHandler(logger, listPinsUC),
		GetPin:             httproutes.NewGetPinHandler(logger, getPinUC),
		CreatePin:          httproutes.NewCreatePinHandler(logger, createPinUC),
		DeletePin:          httproutes.NewDeletePinHandler(logger, deletePinUC),
		SetPinGoing:        httproutes.NewSetPinGoingHandler(logger, setPinGoingUC),
		ReportPin:          httproutes.NewReportPinHandler(logger, reportPinUC),
		ListTrips:          httproutes.NewListTripsHandler(logger, listTripsUC),
		CreateTrip:         httproutes.NewCreateTripHandler(logger, createTripUC),
		UpdateTrip:         httproutes.NewUpdateTripHandler(logger, updateTripUC),
		ListReports:        httproutes.NewListReportsHandler(logger, listReportsUC),
		CreateReport:       httproutes.NewCreateReportHandler(logger, createReportUC),
		CreateUser:         httproutes.NewCreateUserHandler(logger, createUserUC, cfg.IsProd()),
		GetCurrentUser:     httproutes.NewGetCurrentUserHandler(logger),
		CreateSubmission:   httproutes.NewCreateSubmissionHandler(logger, createSubmissionUC),
		GetSubmission:      httproutes.NewGetSubmissionHandler(logger, getSubmissionUC),
		ListSubmissions:    httproutes.NewListSubmissionsHandler(logger, listSubmissionsUC),
		PhotoUpload:        httproutes.NewPhotoUploadHandler(logger, createPhotoUC, photoStore, cfg.PhotosConfig.MaxUploadMB<<20),
		RequireAuth:        authMiddleware.Require,
		LoadAuth:           authMiddleware.Load,
		AllowedOrigins:     cfg.AllowedOriginList(),
		Pages:              pages,
		Fragments:          httproutes.NewFragments(logger, pages),
		MapItems:           httproutes.NewMapItems(logger, pages),
		Item:               httproutes.NewItem(logger, pages),
		Logout:             httproutes.NewLogoutHandler(logger, sessionRepo, cfg.IsProd()),
		UpdateUser:         httproutes.NewUpdateCurrentUserHandler(logger, userRepo),
		MapAssets:          httproutes.NewMapAssetsHandler(cfg.MapDir),
		MapStyle:           httproutes.NewMapStyleHandler(cfg.MapDir, cfg.ReliefPath),
		PMTiles:            httproutes.NewPMTilesHandler(logger, cfg.PMTilesPath),
		Relief:             httproutes.NewPMTilesHandler(logger, cfg.ReliefPath),
		WebDir:             cfg.WebDir,
	})

	server := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           router,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		logger.Info("http server started", slog.String("addr", cfg.Addr()))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", slog.Any("error", err))
			cancel()
		}
	}()

	appCtx, stop := signal.NotifyContext(runCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-appCtx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	//nolint:contextcheck // shutdown must run with a fresh timeout context after appCtx is cancelled
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	logger.Info("http server stopped")

	return nil
}
