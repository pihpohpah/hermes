# Консольные команды (`cmd/server/console.go` и `internal/application/commands`)

## 1. Диспетчер команд (`cmd/server/console.go`)

Разовые и периодические задачи (миграции, сид, загрузка погоды, оценка условий, импорт событий, чистка меток) реализуют
один интерфейс:

```go
type Command interface {
	Run(ctx context.Context) error
}
```

`runCommand` делает три вещи:

1. Оборачивает контекст в `signal.NotifyContext`, чтобы `SIGINT`/`SIGTERM` прерывали команду.
2. Разбирает флаги одним `flag.FlagSet` на все команды (`parseCommandOptions`): `-day`, `-place`, `-activity`,
   `-overwrite`, `-grace_hours` и другие. Строки сразу превращаются в доменные типы, например `models.NewActivity`.
3. По имени команды собирает usecase и команду из `commandDeps` и вызывает `cmd.Run(ctx)`.

### Сокращённый `cmd/server/console.go`

```go
var ErrUnknownCommand = errors.New("unknown command")

const (
	migrateCommand          = "migrate"
	seedCommand             = "seed"
	weatherLoadCommand      = "weather_load"
	conditionsAssessCommand = "conditions_assess"
	pinsCleanupCommand      = "pins_cleanup"
	eventsImportCommand     = "events_import"
	// ...
)

type Command interface {
	Run(ctx context.Context) error
}

func runCommand(ctx context.Context, commandName string, args []string, deps commandDeps) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	opts, err := parseCommandOptions(commandName, args)
	if err != nil {
		return err
	}

	rng, err := commands.ResolveForecastRange(opts.day, time.Now())
	if err != nil {
		return fmt.Errorf("resolve forecast range: %w", err)
	}

	var cmd Command
	switch commandName {
	case migrateCommand:
		cmd = commands.NewMigrateCommand(deps.logger, deps.pool, opts.migrationsDir)
	case seedCommand:
		cmd = commands.NewSeedCommand(deps.logger, deps.places, repository.NewEventRepository(deps.pool))
	case weatherLoadCommand:
		loader := usecase.NewLoadWeatherUseCase(deps.places, deps.weather, deps.provider)
		cmd = commands.NewWeatherLoadCommand(deps.logger, loader, rng, opts.placeID)
	case pinsCleanupCommand:
		pinRepo := repository.NewPinRepository(deps.pool)
		purger := usecase.NewPurgeExpiredPinsUseCase(pinRepo)
		cmd = commands.NewPinsCleanupCommand(deps.logger, purger, opts.pinPurgeGrace)
	// ...
	default:
		return fmt.Errorf("%w: %s", ErrUnknownCommand, commandName)
	}

	if err := cmd.Run(ctx); err != nil {
		return fmt.Errorf("run command %s: %w", commandName, err)
	}

	return nil
}
```

## 2. Команда: тонкая обёртка над usecase (`internal/application/commands`)

Логика живёт в usecase, команда только вызывает его и пишет отчёт в лог. Так один и тот же usecase работает и из
консоли, и из HTTP: `RefreshForecastUseCase` вызывают и `forecast_refresh`, и `POST`-ручка обновления прогноза.

Команда объявляет узкий интерфейс на то, что ей нужно, рядом со своей структурой. Usecase возвращает отчёт
(`WeatherLoadReport`, `AssessReport`, `ImportReport`), команда раскладывает его в структурные поля `slog`. Сбои по
отдельным местам идут в `Warn` и не роняют всю команду.

### Пример: `WeatherLoadCommand` (`conditions.go`)

```go
type weatherLoader interface {
	Do(ctx context.Context, rng models.ForecastRange, placeID string) (usecase.WeatherLoadReport, error)
}

// WeatherLoadCommand downloads hourly weather for all places (or a single one)
// and stores it as raw observations.
type WeatherLoadCommand struct {
	logger  *slog.Logger
	loader  weatherLoader
	rng     models.ForecastRange
	placeID string
}

func NewWeatherLoadCommand(
	logger *slog.Logger,
	loader weatherLoader,
	rng models.ForecastRange,
	placeID string,
) *WeatherLoadCommand {
	return &WeatherLoadCommand{logger: logger, loader: loader, rng: rng, placeID: placeID}
}

func (c *WeatherLoadCommand) Run(ctx context.Context) error {
	report, err := c.loader.Do(ctx, c.rng, c.placeID)
	if err != nil {
		return fmt.Errorf("load weather from %s: %w", c.rng.From.Format(time.RFC3339), err)
	}

	c.logger.Info(
		"weather loaded",
		slog.String("day", report.Day),
		slog.Int("places", report.LoadedPlaces),
		slog.Int("hours", report.LoadedHours),
		slog.Int("failed", len(report.Failed)),
		slog.Duration("took", report.FinishedAt.Sub(report.StartedAt)),
	)
	for _, failure := range report.Failed {
		c.logger.Warn(
			"place weather failed",
			slog.String("place_id", failure.PlaceID),
			slog.String("place", failure.PlaceName),
			slog.String("reason", failure.Reason),
		)
	}

	return nil
}
```

Команда без usecase допустима, если сценарий нужен только из консоли. `SeedCommand` (`seed.go`) сам ходит в
репозитории через узкие интерфейсы `seedPlaceRepo` и `seedEventRepo` и повторно запускается без дублей: события
пишутся через `UpsertBySlug`.

Если у нескольких команд появляется общий шаг, его выносят в неэкспортируемую структуру с узким интерфейсом
репозитория и встраивают в команды (struct embedding). Копировать шаг между файлами не нужно.

## 3. Запросы во внешние сервисы

HTTP-клиенты внешних сервисов лежат в `internal/infrastructure` (`openmeteo`, `eventagent`, `geocoder`, `telegram`), а не
в командах. Команда получает их через usecase. Правила для клиента (`eventagent/client.go`, `geocoder/nominatim.go`):

1. Таймаут задан в `http.Client{Timeout: cfg.Timeout}` или через `context.WithTimeout`.
2. Запрос несёт понятный `User-Agent` (`hermes-events-import/1.0`). Для Nominatim это требование сервиса.
3. Ответ с неожиданным статусом превращается в ошибку с кодом; известные статусы маппятся в доменные ошибки
   (`404` → `models.ErrNotFound`, `409` → `models.ErrConflict`).
4. `resp.Body.Close()` закрывается в `defer`, ошибка закрытия пишется в `Warn`.
5. Тело читается через `io.LimitReader`, чтобы сломанный upstream не съел память.
6. DTO ответа маппится в доменные модели внутри пакета клиента (`toDomain()` в `eventagent/mapping.go`).

Если команда синхронизирует таблицу со списком из внешнего источника и удаляет записи, которых там нет, пустой ответ
источника считается ошибкой. Иначе сбой парсинга на той стороне удалит все строки.

### Пример: `eventagent.Client.GetRun`

```go
func (c *Client) GetRun(ctx context.Context, runID string) (models.ImportedRun, error) {
	if strings.TrimSpace(runID) == "" {
		return models.ImportedRun{}, errors.New("get run: empty run id")
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, c.base()+"/runs/"+url.PathEscape(runID), nil,
	)
	if err != nil {
		return models.ImportedRun{}, fmt.Errorf("build get run request: %w", err)
	}
	req.Header.Set("User-Agent", "hermes-events-import/1.0")

	resp, err := c.cfg.Client.Do(req)
	if err != nil {
		return models.ImportedRun{}, fmt.Errorf("get run request: %w", err)
	}
	defer c.closeBody(resp)

	if resp.StatusCode == http.StatusNotFound {
		return models.ImportedRun{}, fmt.Errorf("get run `%s`: %w", runID, models.ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return models.ImportedRun{}, fmt.Errorf("get run `%s`: unexpected status %d", runID, resp.StatusCode)
	}

	run, err := decodeRun(resp)
	if err != nil {
		return models.ImportedRun{}, fmt.Errorf("decode get run response: %w", err)
	}

	return run, nil
}

func (c *Client) closeBody(resp *http.Response) {
	if err := resp.Body.Close(); err != nil {
		c.logger.Warn("close event-agent response body", slog.Any("error", err))
	}
}
```
