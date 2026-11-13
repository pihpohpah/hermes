# Устройство `cmd` и `infrastructure/config`

## 1. Паттерн: Single-Binary Multi-Mode (`cmd/<app>/main.go`)

Приложение собирается в один бинарный файл и выбирает режим работы по первому аргументу командной строки (`os.Args[1]`):
- `listen` — запуск HTTP и gRPC серверов.
- `<command_name>` — разовое выполнение консольной команды (используется для Kubernetes `CronJob`).

В `main.go` инициализируются только общие для всех режимов инфраструктурные ресурсы:
1. Конфигурация (`config.Load()`).
2. Мульти-обработчик логов `slog.NewMultiHandler` (JSON в `stdout` + `sentryslog` для `Warn`/`Error`/`Fatal`).
3. Подключение к PostgreSQL (`sql.Open` + `db.PingContext` с таймаутом).
4. Репозиторий.

### Пример `cmd/<app>/main.go`

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/getsentry/sentry-go"
	sentryslog "github.com/getsentry/sentry-go/slog"
	_ "github.com/lib/pq" // trigger init

	"example.com/project/internal/infrastructure/config"
	"example.com/project/internal/infrastructure/repository"
)

// @title	Swagger Service API
// @version	1.0.0
func main() {
	ctx := context.Background()

	const minArgsCount = 2
	if len(os.Args) < minArgsCount {
		log.Fatal("Usage: app [listen|console_command_name]")
	}
	command := os.Args[1]

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}
	logger, err := newLogger(ctx, cfg)
	if err != nil {
		log.Fatalf("logger initialization failed: %v", err)
	}

	db, err := sql.Open("postgres", cfg.URL)
	if err != nil {
		log.Fatalf("failed to open postgres connection: %v", err)
	}
	pingCtx, pingCancel := context.WithTimeout(ctx, cfg.PingTimeout)
	if err := db.PingContext(pingCtx); err != nil {
		pingCancel()
		if cErr := db.Close(); cErr != nil {
			log.Printf("postgres close after ping failure: %v", cErr)
		}
		log.Fatalf("failed to ping postgres: %v", err)
	}
	pingCancel()

	repo := repository.NewTrustedNetRepository(db)

	switch command {
	case "listen":
		err = runListen(ctx, logger, cfg, repo, db)
	default:
		err = runCommand(ctx, command, logger, repo)
	}

	if cErr := db.Close(); cErr != nil {
		log.Printf("postgres close: %v", cErr)
	}
	if err != nil {
		log.Fatalf("console run err: %v", err)
	}
}

func newLogger(ctx context.Context, cfg config.Config) (*slog.Logger, error) {
	handlers := []slog.Handler{
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:     cfg.Level,
			AddSource: cfg.AddSource,
		}),
	}

	if cfg.DSN != "" {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn:              cfg.DSN,
			Environment:      cfg.Environment,
			Release:          cfg.Release,
			Debug:            cfg.Debug,
			EnableTracing:    cfg.SampleRate > 0,
			TracesSampleRate: cfg.SampleRate,
		}); err != nil {
			return nil, fmt.Errorf("initialize sentry: %w", err)
		}

		handlers = append(handlers, sentryslog.Option{
			LogLevel:  []slog.Level{slog.LevelWarn, slog.LevelError, sentryslog.LevelFatal},
			AddSource: cfg.AddSource,
		}.NewSentryHandler(ctx))
	}

	return slog.New(slog.NewMultiHandler(handlers...)), nil
}
```

---

## 2. Паттерн: Серверный режим и Graceful Shutdown (`cmd/<app>/listen.go`)

В `listen.go` происходит явная сборка (Pure DI) всех UseCase и транспортных адаптеров:
- Один и тот же экземпляр `UseCase` передается и в HTTP-хендлер, и в gRPC-сервер.
- Оба сервера запускаются в отдельных горутинах и разделяют общий `runCtx`.
- Если один из серверов падает с ошибкой, он вызывает `cancel()`, инициируя остановку всего процесса.
- При получении `SIGINT`/`SIGTERM` (или отмене `runCtx`) вызывается `server.Shutdown(shutdownCtx)` с таймаутом `cfg.ShutdownTimeout`, а `defer grpcSrv.GracefulStop()` останавливает gRPC.

### Пример `cmd/<app>/listen.go`

```go
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"syscall"

	servicev1 "example.com/project-proto/proto/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"example.com/project/docs"
	"example.com/project/internal/application/routes"
	grpcreg "example.com/project/internal/application/routes/grpc"
	httproutes "example.com/project/internal/application/routes/http"
	"example.com/project/internal/application/usecase"
	"example.com/project/internal/infrastructure/config"
	"example.com/project/internal/infrastructure/repository"
)

func runListen(
	ctx context.Context,
	logger *slog.Logger,
	cfg config.Config,
	repo *repository.TrustedNetRepository,
	db *sql.DB,
) error {
	if cfg.SwaggerHost != "" {
		docs.SwaggerInfo.Host = cfg.SwaggerHost
	}

	getBotsUC := usecase.NewGetBotsUseCase(repo)
	getBotsHandler := httproutes.NewGetBotsHandler(logger, getBotsUC)
	healthHandler := httproutes.NewHealthCheckHandler(logger, db)
	router := routes.NewHTTPRouter(getBotsHandler, healthHandler)

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var grpcSrv *grpc.Server
	if cfg.GRPCAddr != "" {
		lis, err := net.Listen("tcp", cfg.GRPCAddr)
		if err != nil {
			return fmt.Errorf("grpc listen: %w", err)
		}
		grpcSrv = grpc.NewServer()
		servicev1.RegisterBotsServiceServer(grpcSrv, grpcreg.NewBotsServer(getBotsUC))
		reflection.Register(grpcSrv)
		go func() {
			logger.Info("grpc server started", "addr", cfg.GRPCAddr)
			if err := grpcSrv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				logger.Error("grpc server failed", "error", err)
				cancel()
			}
		}()

		defer grpcSrv.GracefulStop()
	}

	go func() {
		logger.Info("http server started", slog.String("addr", cfg.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
			cancel()
		}
	}()

	appCtx, stop := signal.NotifyContext(runCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-appCtx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	//nolint:contextcheck
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	logger.Info("http server stopped")

	return nil
}
```

---

## 3. Паттерн: Конфигурация со Squash-композицией (`internal/infrastructure/config`)

Конфигурация разбивается на тематические структуры (`AppConfig`, `DatabaseConfig`, `LoggerConfig`, `SentryConfig`), которые встраиваются в корневую структуру `Config`.

Порядок загрузки:
1. Дефолтные значения в `defaultCfg()`.
2. Опциональный файл `.env` через `koanf`.
3. Переменные окружения ОС.
4. Декодирование через `mapstructure` с `Squash: true`, хуками `StringToTimeDurationHookFunc()` и `TextUnmarshallerHookFunc()`.
5. Валидация через `validator.New().Struct(cfg)`.

### Пример `internal/infrastructure/config/config.go`

```go
package config

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	AppConfig
	DatabaseConfig
	LoggerConfig
	SentryConfig
}

type AppConfig struct {
	Addr              string        `koanf:"APP_ADDR"`
	GRPCAddr          string        `koanf:"GRPC_ADDR"`
	ReadHeaderTimeout time.Duration `koanf:"APP_READ_HEADER_TIMEOUT"`
	ShutdownTimeout   time.Duration `koanf:"APP_SHUTDOWN_TIMEOUT"`
	SwaggerHost       string        `koanf:"SWAGGER_HOST"`
}

type DatabaseConfig struct {
	URL         string        `koanf:"DB_URL" validate:"required"`
	PingTimeout time.Duration `koanf:"DB_PING_TIMEOUT" validate:"required"`
}

type LoggerConfig struct {
	Level     slog.Level `koanf:"LOG_LEVEL"`
	AddSource bool       `koanf:"LOG_ADD_SOURCE"`
}

type SentryConfig struct {
	DSN         string  `koanf:"SENTRY_DSN"`
	Environment string  `koanf:"SENTRY_ENVIRONMENT"`
	Release     string  `koanf:"SENTRY_RELEASE"`
	Debug       bool    `koanf:"SENTRY_DEBUG"`
	SampleRate  float64 `koanf:"SENTRY_TRACES_SAMPLE_RATE"`
}

func Load() (Config, error) {
	k := koanf.New("")

	if _, err := os.Stat(".env"); err == nil {
		if err := k.Load(file.Provider(".env"), dotenv.ParserEnv(".", "=", nil)); err != nil {
			return Config{}, fmt.Errorf("load config file `.env`: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return Config{}, fmt.Errorf("stat config file `.env` err: %w", err)
	}

	if err := k.Load(env.Provider("", ".", nil), nil); err != nil {
		return Config{}, fmt.Errorf("load config from env: %w", err)
	}

	cfg := defaultCfg()
	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{
		DecoderConfig: &mapstructure.DecoderConfig{
			Squash:           true,
			Result:           &cfg,
			WeaklyTypedInput: true,
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				mapstructure.TextUnmarshallerHookFunc(),
			),
		},
	}); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := validator.New().Struct(cfg); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

const (
	defaultReaderTimeout = time.Second * 10
	defaultPingTimeout   = time.Second * 5
)

func defaultCfg() Config {
	return Config{
		AppConfig: AppConfig{
			Addr:              ":8080",
			GRPCAddr:          ":9090",
			ReadHeaderTimeout: defaultReaderTimeout,
		},
		DatabaseConfig: DatabaseConfig{
			PingTimeout: defaultPingTimeout,
		},
		LoggerConfig: LoggerConfig{
			Level: slog.LevelDebug,
		},
	}
}
```
