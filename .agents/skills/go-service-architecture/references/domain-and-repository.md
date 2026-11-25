# Устройство `domain` и `infrastructure/repository`

## 1. Сущности и value objects с конструкторами (`internal/domain/models`)

Пакет `domain` импортирует только stdlib. Доменных пакетов два:

- `models` хранит сущности (`Place`, `Event`, `Pin`, `User`), value objects и общие ошибки;
- `scoring` содержит эвристику оценки условий.

Правила:

1. Сущность — обычная Go-структура со строгими доменными типами вместо голых `string`/`int`: `Type PlaceType`,
   `Layer LayerCode`, `Location Point`.
2. Value object — именованный тип над примитивом (`type Activity string`, `type PlaceType string`) с набором констант.
3. Конструктор `New<Type>(raw) (<Type>, error)` не даёт создать невалидное значение. Ошибку он возвращает через
   `models.Invalid(...)` / `models.Invalidf(...)`, чтобы она совпадала с `errors.Is(err, models.ErrInvalidArgument)`.
4. Общие ошибки лежат в `errors.go`: `ErrInvalidArgument`, `ErrNotFound`, `ErrUnauthorized`, `ErrForbidden`,
   `ErrConflict`. HTTP-слой маппит их в 400, 404, 401, 403 и 409.
5. Сущности (`Place`, `Event`, `Pin`) не несут тегов сериализации: JSON для API собирает `presenter`, строки БД
   разбирает `repository`. Теги допустимы у вспомогательных структур, которые хранятся в `jsonb` или приходят извне
   как есть: `WeatherObservation`, `SourceStatus`, `Photo`, `TelegramIDClaims`.

### Пример value object (`internal/domain/models/activity.go`)

```go
// Activity is the kind of outdoor pursuit a forecast is scored for.
type Activity string

const (
	ActivityHike Activity = "hike"
	ActivitySup  Activity = "sup"
	ActivityRun  Activity = "run"
	ActivityWalk Activity = "walk"
)

// NewActivity parses an activity coming from an HTTP query, CLI flag or a
// database row. Unlike a plain type conversion it rejects unknown values, so a
// typo cannot silently produce a zero-score forecast.
func NewActivity(raw string) (Activity, error) {
	activity := Activity(raw)
	if !activity.Valid() {
		return "", Invalid(fmt.Sprintf(
			"unexpected activity `%s`, expected: hike, sup, run, walk", raw,
		))
	}

	return activity, nil
}
```

### Пример value object со структурой (`internal/domain/models/geo.go`)

```go
type Point struct {
	Lat float64
	Lon float64
}

func NewPoint(lat, lon float64) (Point, error) {
	if math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) {
		return Point{}, Invalid("coordinates must be finite numbers")
	}

	if lat < minLatitude || lat > maxLatitude {
		return Point{}, Invalid(fmt.Sprintf("latitude `%g` must be between -90 and 90", lat))
	}

	if lon < minLongitude || lon > maxLongitude {
		return Point{}, Invalid(fmt.Sprintf("longitude `%g` must be between -180 and 180", lon))
	}

	return Point{Lat: lat, Lon: lon}, nil
}
```

## 2. SQL-репозиторий на pgx (`internal/infrastructure/repository`)

Репозитории работают с PostgreSQL/PostGIS через `pgx/v5` и `*pgxpool.Pool`, без ORM. Схема лежит в `migrations/` и
накатывается командой `migrate`.

Приёмы:

1. В пакете репозитория нет интерфейсов. Конструктор возвращает конкретный указатель (`*PlaceRepository`), а узкие
   интерфейсы объявляет потребитель (`usecase`, `commands`).
2. Запросы только параметризованные. Динамический `WHERE` собирается счётчиком позиции `argPos`, значения уходят в
   `args`, в текст запроса попадают только плейсхолдеры `$n`. Пример: `buildPlaceListQuery` в `places.go`. Нумерацию
   плейсхолдеров проверяет `places_query_test.go`.
3. Прочитанные значения снова проходят через доменные конструкторы. `rows.Scan` пишет в сырые переменные
   (`typeRaw string`, `layerRaw string`), затем `scanPlace` вызывает `models.NewPlaceType`, `models.NewLayerCode`,
   `models.NewPoint`.
4. Курсор закрывается `defer rows.Close()` (в pgx он не возвращает ошибку), после цикла проверяется `rows.Err()`.
5. `pgx.ErrNoRows` превращается в `models.ErrNotFound`, чтобы usecase и HTTP-слой не знали о драйвере.
6. Повторный запуск сида и импорта не плодит дубли: запись идёт через `INSERT ... ON CONFLICT ... DO UPDATE`
   (`EventRepository.UpsertBySlug`).
7. Транзакции открывает `TxManager.Run(ctx, fn)` и кладёт `pgx.Tx` в контекст. Методы, которые должны работать и в
   транзакции, и без неё, берут соединение через `queryWith(ctx, r.pool)`.

### Сокращённый `internal/infrastructure/repository/places.go`

```go
type PlaceRepository struct {
	pool *pgxpool.Pool
}

func NewPlaceRepository(pool *pgxpool.Pool) *PlaceRepository {
	return &PlaceRepository{pool: pool}
}

func (r *PlaceRepository) List(ctx context.Context, filter models.PlaceFilter) ([]models.Place, error) {
	query, args := buildPlaceListQuery(filter)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query places: %w", err)
	}
	defer rows.Close()

	result := make([]models.Place, 0)
	for rows.Next() {
		place, scanErr := scanPlace(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, place)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate places rows: %w", err)
	}

	return result, nil
}

// GetByID resolves a place by UUID or by its unique slug, so both the API
// identifiers and the human-readable URL part work.
func (r *PlaceRepository) GetByID(ctx context.Context, id string) (models.Place, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM places p
		%s
		WHERE p.id::text = $3 OR p.slug = $3
	`, fmt.Sprintf(placeColumns, "NULL::double precision AS distance_m"), latestAssessmentJoin)

	place, err := scanPlace(
		r.pool.QueryRow(ctx, query, string(models.ActivityWalk), scoring.AlgorithmVersion, id),
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Place{}, models.ErrNotFound
		}

		return models.Place{}, fmt.Errorf("query place by id: %w", err)
	}

	return place, nil
}

func buildPlaceListQuery(filter models.PlaceFilter) (string, []any) {
	leading := listLeadingArgs()
	argPos := len(leading) + 1

	var (
		where   []string
		args    = make([]any, 0, argPos)
		orderBy = "p.created_at DESC"
	)

	if filter.Type != nil {
		where = append(where, fmt.Sprintf("p.type = $%d", argPos))
		args = append(args, string(*filter.Type))
		argPos++
	}

	if filter.Layer != nil {
		where = append(where, fmt.Sprintf("p.layer_code = $%d", argPos))
		args = append(args, string(*filter.Layer))
		argPos++
	}
	// ... радиус (PostGIS), категория, полнотекстовый q

	// ... limit, SELECT, WHERE, ORDER BY собираются тем же способом
	return query, append(leading, args...)
}
```

### Разбор строки в доменную модель (`scanPlace`, фрагмент)

```go
type rowScanner interface {
	Scan(dest ...any) error
}

func scanPlace(row rowScanner) (models.Place, error) {
	var (
		id       string
		typeRaw  string
		layerRaw string
		lat      float64
		lon      float64
		// ...
	)

	if err := row.Scan(&id, /* ... */ &typeRaw, &layerRaw, /* ... */ &lat, &lon /* ... */); err != nil {
		return models.Place{}, fmt.Errorf("scan place row: %w", err)
	}

	pt, err := models.NewPlaceType(typeRaw)
	if err != nil {
		return models.Place{}, fmt.Errorf("parse place type from db `%s`: %w", typeRaw, err)
	}
	layer, err := models.NewLayerCode(layerRaw)
	if err != nil {
		return models.Place{}, fmt.Errorf("parse layer code from db `%s`: %w", layerRaw, err)
	}
	loc, err := models.NewPoint(lat, lon)
	if err != nil {
		return models.Place{}, fmt.Errorf("parse place location: %w", err)
	}
	// ...
}
```
