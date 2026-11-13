package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pihpohpah/hermes/internal/application/routes"
	httproutes "github.com/pihpohpah/hermes/internal/application/routes/http"
	"github.com/pihpohpah/hermes/internal/application/usecase"
	"github.com/pihpohpah/hermes/internal/infrastructure/config"
	"github.com/pihpohpah/hermes/internal/infrastructure/openmeteo"
	"github.com/pihpohpah/hermes/internal/infrastructure/repository"
)

const idleTimeout = time.Minute

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

	getForecastUC := usecase.NewGetPlaceForecastUseCase(placeRepo, forecastRepo, weatherRepo, provider)
	refreshForecastUC := usecase.NewRefreshForecastUseCase(placeRepo, forecastRepo, weatherRepo, provider)

	listTripsUC := usecase.NewListTripsUseCase(tripRepo)
	createTripUC := usecase.NewCreateTripUseCase(tripRepo)
	updateTripUC := usecase.NewUpdateTripUseCase(tripRepo)

	listReportsUC := usecase.NewListReportsUseCase(reportRepo)
	createReportUC := usecase.NewCreateReportUseCase(reportRepo)

	router := routes.NewHTTPRouter(logger, routes.Handlers{
		Health:             httproutes.NewHealthCheckHandler(logger, pool),
		ListPlaces:         httproutes.NewListPlacesHandler(logger, listPlacesUC),
		GetPlace:           httproutes.NewGetPlaceHandler(logger, getPlaceUC),
		CreatePlace:        httproutes.NewCreatePlaceHandler(logger, createPlaceUC),
		GetPlaceForecast:   httproutes.NewGetPlaceForecastHandler(logger, getForecastUC),
		RefreshForecast:    httproutes.NewRefreshForecastHandler(logger, refreshForecastUC),
		ListTrips:          httproutes.NewListTripsHandler(logger, listTripsUC),
		CreateTrip:         httproutes.NewCreateTripHandler(logger, createTripUC),
		UpdateTrip:         httproutes.NewUpdateTripHandler(logger, updateTripUC),
		ListReports:        httproutes.NewListReportsHandler(logger, listReportsUC),
		CreateReport:       httproutes.NewCreateReportHandler(logger, createReportUC),
		PlacesListFragment: httproutes.NewPlacesListFragmentHandler(logger, listPlacesUC),
		PlaceCardFragment:  httproutes.NewPlaceCardFragmentHandler(logger, getPlaceUC),
		IndexPage:          httproutes.NewIndexPageHandler(logger),
		MapPage:            httproutes.NewMapPageHandler(cfg.MapDir),
		MapAssets:          httproutes.NewMapAssetsHandler(cfg.MapDir),
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
