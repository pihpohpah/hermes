---
name: go-service-architecture
description: Архитектурный стандарт и паттерны проектирования Go-сервисов (Clean Architecture, DDD-lite, HTTP/gRPC/CLI, PostgreSQL, koanf, slog). Используй при создании новых сервисов, добавлении фич, роутов, консольных команд, репозиториев и рефакторинге.
---

# Архитектурный шаблон Go-сервиса

Этот скилл задает единый стандарт проектирования и написания кода для микросервисов на Go. Все решения опираются на стандартную библиотеку (`net/http`, `database/sql`, `log/slog`), явную сборку зависимостей (Pure DI) и чистое разделение слоев (`cmd`, `domain`, `application`, `infrastructure`).

---

## 1. Общие правила и стиль кода (Code Style & Guidelines)

1. **Минимум комментариев**:
   - Не пиши очевидные комментарии, дублирующие сигнатуру функции или код.
   - Имена пакетов, типов, функций и переменных должны быть самодокументируемыми.
   - Оставляй комментарии **только** для:
     - аннотаций кодогенерации и документации (`// @Summary ...` для Swagger);
     - пояснения неочевидных сайд-эффектов импортов (`_ "github.com/lib/pq" // trigger init`, `_ "net/http/pprof" // trigger init`);
     - директив линтера (`//nolint:contextcheck`) или критичных инвариантов.
2. **Строгое направление зависимостей**:
   - `domain` -> не импортирует ничего из других слоев проекта (только stdlib).
   - `application` -> импортирует `domain` (а в `presenter` и `routes/grpc` — сгенерированные proto-контракты). Никогда не импортирует `infrastructure`.
   - `infrastructure` -> импортирует `domain` и внешние драйверы/библиотеки.
   - `cmd` -> **Composition Root**, единственное место, где `infrastructure` связывается с `application`.
3. **Интерфейсы на стороне потребителя (Consumer-Driven Interfaces)**:
   - Не создавай гигантские интерфейсы в `domain` или `infrastructure`.
   - Объявляй узкие неэкспортируемые (или локальные для пакета) интерфейсы прямо в том файле, где они используются (`usecase`, `commands`, `routes/http`, `routes/grpc`).
4. **Parse, don't validate**:
   - Входящие примитивы (`string`, `int`, proto-enum) на границе приложения (в HTTP/gRPC хендлерах и при чтении из БД в репозитории) сразу преобразуй в доменные Value Objects через конструкторы (`NewSource`, `NewIPVersion`).
   - Внутрь `usecase` и `repository` передавай уже валидные доменные типы.
5. **Ошибки и контекст**:
   - Первым аргументом любых I/O и бизнес-операций всегда идет `ctx context.Context`.
   - Оборачивай ошибки через `fmt.Errorf("действие: %w", err)` с четким контекстом, без лишнего шума.
   - Закрытие ресурсов (`rows.Close()`, `resp.Body.Close()`, `db.Close()`) всегда проверяй на ошибку в `defer`.
6. **Форматирование и импорты**:
   - Максимальная длина строки — `120` символов (`golines`).
   - Строгий порядок групп импортов (`gci`): 1) стандартная библиотека, 2) внешние пакеты, 3) пакеты текущего модуля.
   - Форматирование по правилам `gofumpt`.

---

## 2. Структура директорий проекта

```text
.
├── cmd/
│   └── <app-name>/
│       ├── main.go          # Точка входа, загрузка конфига, логгера, БД и выбор режима
│       ├── listen.go        # Сборка и запуск HTTP + gRPC серверов, graceful shutdown
│       └── console.go       # Диспетчер CLI/CronJob команд
├── internal/
│   ├── domain/
│   │   └── <aggregate>/     # Доменные сущности (Entities) и Value Objects с конструкторами
│   ├── application/
│   │   ├── usecase/         # Сценарии чтения/записи для синхронных API
│   │   ├── commands/        # Фоновые CLI/Cron задачи (сбор данных, батч-синк)
│   │   ├── presenter/       # Мапперы Domain <-> HTTP DTO / Protobuf
│   │   └── routes/
│   │       ├── router.go    # Инициализация http.ServeMux (/health, /metrics, /swagger/, /api/...)
│   │       ├── http/        # HTTP-хендлеры и обобщенный APIResponseWriter[T]
│   │       └── grpc/        # Реализация сгенерированных gRPC-сервисов
│   └── infrastructure/
│       ├── config/          # Конфигурация через koanf + validator (.env + ENV)
│       └── repository/      # SQL-репозитории поверх стандартного database/sql
└── docs/                    # Автогенерируемая Swagger-документация (swag init)
```

---

## 3. Архитектурные паттерны по слоям (Справочники с кодом)

При реализации конкретного компонента открой соответствующий справочник в `references/`:

| Компонент / Задача | Файл с шаблоном и примерами кода |
| :--- | :--- |
| Точка входа `cmd`, логгер (`slog` + Sentry), конфигурация (`koanf`), Graceful Shutdown | [cmd-and-config.md](<references/cmd-and-config.md>) |
| Доменные модели, Value Objects со Smart Constructors и SQL-репозиторий (`database/sql`) | [domain-and-repository.md](<references/domain-and-repository.md>) |
| Use Cases, Presenter (DTO маппинг), HTTP роутер, Generic `APIResponseWriter` и gRPC сервер | [routes-usecase-presenter.md](<references/routes-usecase-presenter.md>) |
| Консольные команды (CLI / K8s CronJobs), переиспользование логики через struct embedding | [console-commands.md](<references/console-commands.md>) |

---

## 4. Чек-лист при добавлении новой функциональности

### Как добавить новый API-эндпоинт (HTTP и/или gRPC):
1. **Domain**: Проверь наличие нужных сущностей и Value Objects в `internal/domain/<domain_name>/`. При необходимости добавь тип и конструктор `New<Type>(raw) (<Type>, error)`.
2. **Repository**: Добавь метод в `internal/infrastructure/repository/<repo>.go` (работает только с доменными моделями и `context.Context`).
3. **UseCase**: Создай структуру сценария в `internal/application/usecase/<action>.go`, объяви локальный интерфейс `repo` с нужным методом и реализуй метод `Do(ctx, ...)`.
4. **Presenter**: Добавь функцию маппинга доменной модели в JSON DTO (`ToHTTP`) и/или Protobuf (`ToProto`) в `internal/application/presenter/`.
5. **Routes**:
   - Для HTTP: создай хендлер в `internal/application/routes/http/`, использующий `APIResponseWriter[T]`, провалидируй параметры через конструкторы `domain`, добавь Swagger-аннотации и зарегистрируй маршрут в `internal/application/routes/router.go`.
   - Для gRPC: добавь метод в `internal/application/routes/grpc/`, смаппив ошибки валидации в `codes.InvalidArgument`, а ошибки исполнения — в `codes.Internal`.
6. **Wiring (`cmd`)**: Свяжи репозиторий, usecase и хендлеры в `cmd/<app-name>/listen.go`.

### Как добавить новую консольную команду (CronJob):
1. Создай структуру команды в `internal/application/commands/<cmd_name>.go` с методом `Run(ctx context.Context) error`.
2. Если команде нужна общая логика (например, батч-синхронизация с БД), вынеси ее во встраиваемую структуру (struct embedding) с узким интерфейсом репозитория.
3. Зарегистрируй константу имени команды и ветку `case` в `cmd/<app-name>/console.go`.
