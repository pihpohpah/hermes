package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	httproutes "github.com/pihpohpah/hermes/internal/application/routes/http"
)

const (
	defaultAddr      = ":8080"
	defaultMapDir    = "./web/map"
	defaultArchive   = "./web/static/pmtiles/primorsky-krai-z15.pmtiles"
	defaultRelief    = "./web/static/pmtiles/primorsky-relief.pmtiles"
	defaultDem       = "./web/static/pmtiles/primorsky-dem.pmtiles"
	shutdownTimeout  = 10 * time.Second
	readHeaderTimout = 10 * time.Second
	idleTimeout      = time.Minute
)

// Отдельный сервер карты: поднимает страницу /map и архив PMTiles, не требуя
// Postgres и остальной бекенд. Нужен, чтобы фронт карты и бекенд разрабатывались
// независимо.
func main() {
	addr := flag.String("addr", envOr("MAP_ADDR", defaultAddr), "адрес прослушивания")
	mapDir := flag.String("map-dir", envOr("MAP_DIR", defaultMapDir), "каталог со страницей карты")
	archive := flag.String("archive", envOr("PMTILES_PATH", defaultArchive), "путь к PMTiles-архиву")
	relief := flag.String("relief", envOr("RELIEF_PATH", defaultRelief), "путь к PMTiles-архиву рельефа")
	dem := flag.String("dem", envOr("DEM_PATH", defaultDem), "путь к PMTiles-архиву DEM (terrarium)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	mux := http.NewServeMux()
	mapPage := httproutes.NewMapPageHandler(*mapDir)
	mapAssets := httproutes.NewMapAssetsHandler(*mapDir)
	mux.Handle("GET /map/{$}", mapPage)
	mux.Handle("GET /map/", mapAssets)

	pmtilesHandler := httproutes.NewPMTilesHandler(logger, *archive)
	mux.Handle("GET /api/map/region.pmtiles", pmtilesHandler)
	mux.Handle("HEAD /api/map/region.pmtiles", pmtilesHandler)

	// Оба режима (город/природа) отдаются всегда: переключение происходит на
	// клиенте. Архив рельефа подключается автоматически, если он собран;
	// без него кнопка «Природа» на странице просто выключена.
	reliefReady := false
	reliefInfo, statErr := os.Stat(*relief)
	if statErr == nil {
		reliefReady = true
		reliefHandler := httproutes.NewPMTilesHandler(logger, *relief)
		mux.Handle("GET /api/map/relief.pmtiles", reliefHandler)
		mux.Handle("HEAD /api/map/relief.pmtiles", reliefHandler)
		logger.Info("relief mode enabled",
			slog.String("archive", *relief),
			slog.Int64("size_bytes", reliefInfo.Size()),
		)
	} else {
		logger.Warn("relief archive not found, nature mode will be hidden",
			slog.String("path", *relief),
			slog.String("hint", "mise run map-relief (или map-relief-local)"),
		)
	}

	// DEM в terrarium-RGB: нужен MapLibre для объёма (terrain) и отмывки
	// (hillshade из raster-dem). Без него природа рисуется плоско, но работает.
	demReady := false
	if demInfo, statErr := os.Stat(*dem); statErr == nil {
		demReady = true
		demHandler := httproutes.NewPMTilesHandler(logger, *dem)
		mux.Handle("GET /api/map/dem.pmtiles", demHandler)
		mux.Handle("HEAD /api/map/dem.pmtiles", demHandler)
		logger.Info("dem terrain enabled",
			slog.String("archive", *dem),
			slog.Int64("size_bytes", demInfo.Size()),
		)
	} else {
		logger.Warn("dem archive not found, nature mode will be flat (no terrain)",
			slog.String("path", *dem),
			slog.String("hint", "mise run map-relief (или map-relief-local)"),
		)
	}

	// Стиль отдаём с поправкой на наличие архивов: без рельефа/DEM их
	// источники и слои вырезаются, иначе MapLibre будет 404-ить.
	styleHandler := httproutes.NewMapStyleHandler(*mapDir, reliefReady, demReady)
	mux.Handle("GET /map/style.json", styleHandler)

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/map/", http.StatusFound)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	server := &http.Server{
		Addr:              *addr,
		Handler:           withCORS(withRequestLog(logger, mux)),
		ReadHeaderTimeout: readHeaderTimout,
		IdleTimeout:       idleTimeout,
	}

	if _, err := os.Stat(*archive); err != nil {
		logger.Warn("pmtiles archive not found, map will show a setup hint",
			slog.String("path", *archive), slog.Any("error", err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("map server started",
			slog.String("addr", *addr),
			slog.String("map", "http://"+strings.TrimPrefix(*addr, ":")+"/map/"),
			slog.String("archive", *archive),
			slog.Bool("relief", reliefReady),
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("map server failed: %v", err)
		}
	}()

	<-ctx.Done()
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)

	if err := server.Shutdown(shutdownCtx); err != nil {
		cancel()
		log.Fatalf("graceful shutdown failed: %v", err)
	}
	cancel()
	logger.Info("map server stopped")
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
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
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("http request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.String("range", r.Header.Get("Range")),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Range")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)

			return
		}

		next.ServeHTTP(w, r)
	})
}
