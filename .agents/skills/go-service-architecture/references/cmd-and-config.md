# Устройство `cmd` и `infrastructure/config`

## 1. Один бинарник, несколько режимов (`cmd/server/main.go`)

Сервис собирается в один бинарник, режим выбирается первым аргументом (`os.Args[1]`):

- `listen` (по умолчанию, если аргументов нет) поднимает HTTP-сервер;
- `<command_name>` разово выполняет консольную команду: `migrate`, `seed`, `weather_load` и другие. Список и разбор флагов
  лежат в `cmd/server/console.go`.

`main.go` создаёт только ресурсы, общие для всех режимов:

1. Конфигурацию: `config.Load()`.
2. Логгер: `slog.NewMultiHandler` из базового обработчика (JSON в проде, текст локально) и `sentryslog` для
   `Warn`/`Error`/`Fatal`, если задан `SENTRY_DSN`.
3. Пул соединений PostgreSQL: `pgxpool.New` и `pool.Ping` с таймаутом `cfg.PingTimeout`.
4. Репозитории и провайдер погоды, которые нужны и серверу, и командам.

Отдельный бинарник `cmd/mapserver` отдаёт карту без Postgres, для отладки стиля.

### Сокращённый `cmd/server/main.go`

```go
func main() {
	ctx := context.Background()

	command := "listen"
	args := []string{}
	const minArgsCount = 2
	if len(os.Args) >= minArgsCount {
		command = os.Args[1]
		args = os.Args[minArgsCount:]
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}

	logger, err := newLogger(ctx, cfg)
	if err != nil {
		log.Fatalf("logger initialization failed: %v", err)
	}

	pool, err := pgxpool.New(ctx, cfg.URL)
	if err != nil {
		log.Fatalf("failed to create pgx pool: %v", err)
	}

	pingCtx, pingCancel := context.WithTimeout(ctx, cfg.PingTimeout)
	if err := pool.Ping(pingCtx); err != nil {
		pingCancel()
		pool.Close()
		log.Fatalf("failed to ping postgres: %v", err)
	}
	pingCancel()

	placeRepo := repository.NewPlaceRepository(pool)
	weatherRepo := repository.NewWeatherRepository(pool)
	// ... остальные репозитории и провайдер погоды

	switch command {
	case "listen":
		err = runListen(ctx, logger, cfg, pool, placeRepo, weatherRepo, forecastRepo, tripRepo, reportRepo, provider)
	default:
		err = runCommand(ctx, command, args, deps)
	}

	pool.Close()
	if err != nil {
		log.Fatalf("run %s failed: %v", command, err)
	}
}

func newLogger(ctx context.Context, cfg config.Config) (*slog.Logger, error) {
	opts := &slog.HandlerOptions{
		Level:     cfg.Level,
		AddSource: cfg.AddSource,
	}

	var baseHandler slog.Handler
	if cfg.IsProd() {
		baseHandler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		baseHandler = slog.NewTextHandler(os.Stdout, opts)
	}

	handlers := []slog.Handler{baseHandler}

	if cfg.DSN != "" {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn:              cfg.DSN,
			Environment:      cfg.Environment,
			Release:          cfg.Release,
			Debug:            cfg.Debug,
			EnableTracing:    cfg.TracesSampleRate > 0,
			TracesSampleRate: cfg.TracesSampleRate,
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

## 2. Серверный режим и graceful shutdown (`cmd/server/listen.go`)

`runListen` собирает зависимости руками (Pure DI):

- создаёт usecase-ы из репозиториев: `usecase.NewGetPlaceUseCase(placeRepo)`;
- создаёт хендлеры из usecase-ов: `httproutes.NewGetPlaceHandler(logger, getPlaceUC)`;
- передаёт все хендлеры одной структурой `routes.Handlers` в `routes.NewHTTPRouter`.

Сервер работает в отдельной горутине. Если `ListenAndServe` падает, горутина вызывает `cancel()` и процесс
останавливается. По `SIGINT`/`SIGTERM` вызывается `server.Shutdown` с новым контекстом и таймаутом
`cfg.ShutdownTimeout`: контекст приложения к этому моменту уже отменён.

### Сокращённый `cmd/server/listen.go`

```go
func runListen(
	ctx context.Context,
	logger *slog.Logger,
	cfg config.Config,
	pool *pgxpool.Pool,
	placeRepo *repository.PlaceRepository,
	// ... остальные репозитории
	provider openmeteo.HourlyProvider,
) error {
	listPlacesUC := usecase.NewListPlacesUseCase(placeRepo)
	getPlaceUC := usecase.NewGetPlaceUseCase(placeRepo)
	// ... остальные usecase-ы

	router := routes.NewHTTPRouter(logger, routes.Handlers{
		Health:     httproutes.NewHealthCheckHandler(logger, pool),
		ListPlaces: httproutes.NewListPlacesHandler(logger, listPlacesUC),
		GetPlace:   httproutes.NewGetPlaceHandler(logger, getPlaceUC),
		// ...
		WebDir: cfg.WebDir,
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
```

## 3. Конфигурация из встроенных структур (`internal/infrastructure/config`)

Конфигурация разбита на тематические структуры (`AppConfig`, `DatabaseConfig`, `OpenMeteoConfig`, `PinsConfig`,
`SentryConfig` и другие), которые встраиваются в корневую `Config`. Поэтому в коде пишется `cfg.URL`, а не
`cfg.DatabaseConfig.URL`.

Порядок загрузки:

1. Значения по умолчанию из `defaultCfg()`.
2. Файл `.env`, если он есть, через `koanf`.
3. Переменные окружения.
4. Декодирование через `mapstructure` с `Squash: true` и хуками `StringToTimeDurationHookFunc()` и
   `TextUnmarshallerHookFunc()`. Второй хук разбирает `LOG_LEVEL` прямо в `slog.Level`.
5. Проверка через `validator.New().Struct(cfg)`.

Новая переменная окружения появляется в трёх местах: поле с тегом `koanf` (и `validate`, если нужно), значение по
умолчанию в `defaultCfg()` и строка в `.env.example`.

### Сокращённый `config.go`

```go
type Config struct {
	AppConfig
	DatabaseConfig
	OpenMeteoConfig
	LoggerConfig
	SentryConfig
	PinsConfig
	// ...
}

type DatabaseConfig struct {
	URL         string        `koanf:"DATABASE_URL" validate:"required"`
	PingTimeout time.Duration `koanf:"DB_PING_TIMEOUT" validate:"required"`
}

type OpenMeteoConfig struct {
	Mode     WeatherMode   `koanf:"WEATHER_MODE" validate:"required,oneof=real mock"`
	BaseURL  string        `koanf:"OPENMETEO_BASE" validate:"required,url"`
	Timeout  time.Duration `koanf:"OPENMETEO_TIMEOUT" validate:"required"`
	Attempts int           `koanf:"OPENMETEO_ATTEMPTS" validate:"required,gte=1,lte=5"`
	// ...
}

type LoggerConfig struct {
	Level     slog.Level `koanf:"LOG_LEVEL"`
	AddSource bool       `koanf:"LOG_ADD_SOURCE"`
}

func Load() (Config, error) {
	k := koanf.New("")

	if _, err := os.Stat(".env"); err == nil {
		if err := k.Load(file.Provider(".env"), dotenv.ParserEnv(".", "=", nil)); err != nil {
			return Config{}, fmt.Errorf("load config file `.env`: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return Config{}, fmt.Errorf("stat config file `.env`: %w", err)
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

func defaultCfg() Config {
	return Config{
		DatabaseConfig: DatabaseConfig{
			URL:         defaultDatabaseURL,
			PingTimeout: defaultPingTimeout,
		},
		LoggerConfig: LoggerConfig{
			Level: slog.LevelDebug,
		},
		// ...
	}
}
```
