---
name: go-service-architecture
description: Архитектура и паттерны Go-кода hermes (слои cmd/domain/application/infrastructure, Pure DI, pgx/PostgreSQL, koanf, slog, HTTP на net/http, консольные команды). Используй при добавлении фич, роутов, консольных команд, репозиториев и при рефакторинге.
---

# Архитектура Go-кода hermes

Сервис собран на stdlib (`net/http`, `log/slog`) и `pgx/v5` для PostgreSQL/PostGIS. Зависимости собираются руками
(Pure DI), код разделён на слои `cmd`, `domain`, `application`, `infrastructure`.

## 1. Общие правила

1. Комментарии.
   - Не пиши комментарии, которые повторяют сигнатуру или код. Имена пакетов, типов и функций должны объяснять себя
     сами.
   - Комментарий нужен, когда код не объясняет «почему»: неочевидный инвариант, побочный эффект импорта
     (`_ "net/http/pprof" // trigger init`), директива линтера с причиной
     (`//nolint:contextcheck // shutdown must run with a fresh timeout context`).
2. Направление зависимостей.
   - `domain` не импортирует другие слои проекта, только stdlib.
   - `application` импортирует `domain` и никогда `infrastructure`.
   - `infrastructure` импортирует `domain` и внешние драйверы и клиенты.
   - `cmd` — composition root, единственное место, где `infrastructure` связывается с `application`.
3. Интерфейсы объявляет потребитель.
   - Не заводи общих интерфейсов в `domain` или `infrastructure`.
   - Объявляй узкий интерфейс в том файле, где он используется (`usecase`, `commands`, `routes/http`), только с
     нужными методами.
4. Parse, don't validate.
   - Примитивы на границе (HTTP-параметры, флаги CLI, строки из БД) сразу превращай в доменные value objects
     конструкторами: `models.NewActivity`, `models.NewPlaceType`, `models.NewPoint`.
   - В `usecase` и `repository` передавай уже валидные доменные типы.
5. Ошибки и контекст.
   - Первый аргумент любой I/O- и бизнес-операции — `ctx context.Context`.
   - Оборачивай ошибки через `fmt.Errorf("действие: %w", err)`.
   - Ошибку валидации создавай через `models.Invalid(...)`, а «не найдено» возвращай как `models.ErrNotFound`. HTTP-слой
     сам переведёт их в статус.
   - Закрывай ресурсы в `defer`. Если `Close()` возвращает ошибку (`resp.Body.Close()`), пиши её в `Warn`.
6. Оформление. Линтер в репозитории не настроен, правила соблюдаются вручную.
   - Код проходит `gofmt`, строки не длиннее 120 символов.
   - Импорты тремя группами: stdlib, внешние пакеты, пакеты `github.com/pihpohpah/hermes`.

## 2. Структура каталогов

```text
.
├── cmd/
│   ├── server/
│   │   ├── main.go          # конфиг, логгер, pgxpool, репозитории, выбор режима
│   │   ├── listen.go        # сборка usecase-ов и хендлеров, HTTP-сервер, graceful shutdown
│   │   └── console.go       # диспетчер консольных команд и разбор их флагов
│   └── mapserver/           # отдельный сервер карты без Postgres, для отладки стиля
├── internal/
│   ├── domain/
│   │   ├── models/          # сущности, value objects с конструкторами, ошибки ErrNotFound и др.
│   │   └── scoring/         # эвристика оценки условий
│   ├── application/
│   │   ├── usecase/         # сценарии для HTTP и консольных команд
│   │   ├── commands/        # консольные команды: migrate, seed, weather_load, events_import...
│   │   ├── presenter/       # доменные модели → JSON DTO
│   │   └── routes/
│   │       ├── router.go    # http.ServeMux, middleware, регистрация маршрутов
│   │       └── http/        # JSON-хендлеры, страницы и htmx-фрагменты, APIResponseWriter[T]
│   └── infrastructure/
│       ├── config/          # koanf + validator (.env + ENV)
│       ├── repository/      # SQL-репозитории на pgx, TxManager
│       ├── openmeteo/       # клиент погоды и demo-провайдер
│       ├── eventagent/      # клиент сервиса скрапинга событий
│       ├── geocoder/        # Nominatim
│       ├── telegram/        # Bot API и проверка id_token
│       └── photostore/      # хранение загруженных фото на диске
├── migrations/              # SQL-миграции, накатываются командой migrate
├── web/
│   ├── templates/           # шаблоны templ и сгенерированные *_templ.go
│   ├── static/              # CSS, JS-модули, архивы карты, загрузки
│   └── map/                 # стиль карты, MapLibre, глифы
└── docs/                    # база знаний проекта
```

## 3. Справочники по слоям

Перед работой над компонентом открой нужный справочник в `references/`:

| Компонент | Справочник |
| :--- | :--- |
| `cmd/server`, логгер (`slog` + Sentry), конфигурация (`koanf`), graceful shutdown | [cmd-and-config.md](<references/cmd-and-config.md>) |
| Доменные модели, value objects с конструкторами, SQL-репозиторий на pgx | [domain-and-repository.md](<references/domain-and-repository.md>) |
| Usecase, presenter, HTTP-роутер, `APIResponseWriter[T]`, страницы и фрагменты | [routes-usecase-presenter.md](<references/routes-usecase-presenter.md>) |
| Консольные команды и клиенты внешних сервисов | [console-commands.md](<references/console-commands.md>) |

## 4. Чек-листы

### Новый HTTP-эндпоинт

1. Domain. Проверь, что нужные сущности и value objects есть в `internal/domain/models/`. Если нет, добавь тип и
   конструктор `New<Type>(raw) (<Type>, error)`.
2. Repository. Добавь метод в `internal/infrastructure/repository/<entity>.go`. Он принимает `context.Context` и
   доменные типы и возвращает доменные модели.
3. Usecase. Добавь структуру в `internal/application/usecase/<entity>.go` с узким интерфейсом на нужный метод
   репозитория, конструктором `New<Name>UseCase` и методом `Do(ctx, ...)`.
4. Presenter. Добавь DTO и функцию `<Model>ToHTTP` в `internal/application/presenter/`.
5. Routes. Создай хендлер в `internal/application/routes/http/` на `APIResponseWriter[T]`, разбери параметры доменными
   конструкторами, переводи ошибки через `statusCode(err)`. Добавь поле в `routes.Handlers` и маршрут в
   `routes/router.go`, при необходимости под `requireAuth` или `loadAuth`.
6. Wiring. Свяжи репозиторий, usecase и хендлер в `cmd/server/listen.go`.

### Новая консольная команда

1. Если логика нужна не только из консоли, вынеси её в usecase.
2. Создай команду в `internal/application/commands/<name>.go`: узкий интерфейс на usecase, конструктор
   `New<Name>Command` и метод `Run(ctx context.Context) error`, который пишет отчёт в `slog`.
3. Добавь константу имени и ветку `case` в `cmd/server/console.go`. Новые флаги добавляй в `parseCommandOptions`.
4. Если команду нужно запускать из `mise`, добавь таску в `mise.toml`.
