package main

import (
	"context"
	_ "embed"
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
	defaultWebDir    = "./web/static"
	defaultArchive   = "./web/static/pmtiles/primorsky-krai-z15.pmtiles"
	defaultRelief    = "./web/static/pmtiles/primorsky-relief.pmtiles"
	shutdownTimeout  = 10 * time.Second
	readHeaderTimout = 10 * time.Second
	idleTimeout      = time.Minute
)

// devPage — отладочная страница стиля. Вшита в бинарник, чтобы основной
// сервер, раздающий web/map, её не видел.
//
//go:embed dev.html
var devPage []byte

// Отдельный сервер карты: поднимает отладочную страницу /map/, стиль, архивы
// PMTiles и статику приложения, не требуя Postgres и остального бекенда.
// Страница рисует карту тем же модулем, что и продукт, поэтому стиль и токены
// отлаживаются здесь и без расхождений попадают в приложение.
func main() {
	addr := flag.String("addr", envOr("MAP_ADDR", defaultAddr), "адрес прослушивания")
	mapDir := flag.String("map-dir", envOr("MAP_DIR", defaultMapDir), "каталог ассетов карты")
	webDir := flag.String("web-dir", envOr("WEB_DIR", defaultWebDir), "каталог статики приложения")
	archive := flag.String("archive", envOr("PMTILES_PATH", defaultArchive), "путь к PMTiles-архиву")
	relief := flag.String("relief", envOr("RELIEF_PATH", defaultRelief), "путь к PMTiles-архиву рельефа")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /map/{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(devPage)
	})
	mux.Handle("GET /map/", httproutes.NewMapAssetsHandler(*mapDir))
	mux.Handle("GET /map/style.json", httproutes.NewMapStyleHandler(*mapDir, *relief))

	pmtilesHandler := httproutes.NewPMTilesHandler(logger, *archive)
	mux.Handle("GET /api/map/region.pmtiles", pmtilesHandler)
	mux.Handle("HEAD /api/map/region.pmtiles", pmtilesHandler)

	reliefHandler := httproutes.NewPMTilesHandler(logger, *relief)
	mux.Handle("GET /api/map/relief.pmtiles", reliefHandler)
	mux.Handle("HEAD /api/map/relief.pmtiles", reliefHandler)

	// Статика приложения: модули карты, CSS с токенами --map-*, спрайт иконок.
	mux.Handle("GET /", http.FileServer(http.Dir(*webDir)))

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
		logger.Warn("pmtiles archive not found, map will fail to load",
			slog.String("path", *archive), slog.String("hint", "mise run map-download"))
	}
	if _, err := os.Stat(*relief); err != nil {
		logger.Warn("relief archive not found, nature mode falls back to city layers",
			slog.String("path", *relief), slog.String("hint", "mise run map-relief (или map-relief-local)"))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("map server started",
			slog.String("addr", *addr),
			slog.String("map", "http://"+strings.TrimPrefix(*addr, ":")+"/map/"),
			slog.String("archive", *archive),
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
