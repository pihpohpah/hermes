# hermes

## О сервисе

Приложение для планирования активного отдыха ВЫХОДИ (рабочее название).

Ответственная команда: PihPohPahPuh.

## Стек

- Backend: Go 1.27, PostgreSQL, PostGIS
- Frontend: серверный рендер templ + htmx + Alpine.js (без React); карта — MapLibre GL JS + PMTiles (векторные тайлы, офлайн-архив региона)

## Локальный запуск

С нуля до работающего сайта:

```bash
mise install          # go, esbuild
mise run data-load    # один раз на свежую базу: Postgres, миграции, сид, mock-погода
mise run map-ensure   # по желанию: архивы карты (нужен Docker, долго)
mise run dev          # дальше — только эта команда
```

`dev` выполняет цепочку тасок:

1. `bootstrap` — `.env` из `.env.example`, `go mod download`;
2. `db-up` — PostGIS в Docker (`postgis/postgis:18-3.6`, контейнер
   `POSTGRES_CONTAINER`): работающий переиспользуется, остановленный
   запускается с прежними данными, иначе создаётся новый. Если на
   `POSTGRES_PORT` уже отвечает другая база, используется она;
3. `migrate-up` — `go run ./cmd/server migrate`: идемпотентные миграции из
   `migrations/` по `DATABASE_URL`, без psql и `docker exec`;
4. `go run ./cmd/server listen`.

Параллельно с шагами 1–3 `assets` собирает JS/CSS-бандлы.

`data-load` проходит ту же цепочку до `migrate-up`, затем запускает `seed`
(5 мест вокруг Владивостока; повторный запуск обновляет их, а не дублирует),
погоду в `mock`-режиме и `conditions_assess`. В `dev` его нет, чтобы каждый
запуск не перетирал погоду демо-данными.

Сайт: **http://localhost:8080**. Вход по имени, одним шагом, сессия в
cookie. Полезные страницы: `/health`, `/catalog`, `/now`, `/rules`, `/profile`.

Нужен Docker или своя PostGIS. Свою укажите в `DATABASE_URL` и `POSTGRES_PORT`:
`db-up` увидит открытый порт и не станет поднимать контейнер. Образы
`postgis/postgis` собираются только под amd64, поэтому на Apple Silicon
контейнер работает через эмуляцию (`--platform linux/amd64`).

Офлайн-архивы карты (`web/static/pmtiles/*.pmtiles`) в репозиторий не входят.
`map-ensure` собирает недостающие: векторную карту (5–10 мин), рельеф с
горизонталями (около 2 ч, качается DEM всего края) и мировую воду. Для воды
шейпы Natural Earth `ne_50m_ocean` и `ne_50m_lakes` нужно заранее скачать с
naturalearthdata.com и распаковать в `.cache/map/water/`. Без
векторного архива подложки нет («Карта не загрузилась»), без рельефа «Природа»
рисуется городской подложкой. Подробности — в [MAP.md](MAP.md).

Основные переменные окружения (остальные — в `.env.example` и
`internal/infrastructure/config/config.go`, модерация меток — в
[docs/api.md](docs/api.md#радар-сейчас)):

| Переменная | По умолчанию | На что влияет |
| :--- | :--- | :--- |
| `DATABASE_URL` | `postgres://hermes:***@localhost:5432/hermes?sslmode=disable` | подключение к БД |
| `PORT` | `8080` | порт HTTP-сервера |
| `WEB_DIR` | `./web/static` | корень статики (CSS/JS/загруженные фото) |
| `WEATHER_MODE` | `real` | `mock` — демо-данные без сети; `real` — запросы к Open-Meteo |
| `PHOTOS_STORAGE_DIR` | `<WEB_DIR>/uploads` | куда писать загруженные фото |
| `PHOTOS_MAX_UPLOAD_MB` | `10` | лимит одной загрузки фото |

`POSTGRES_CONTAINER`, `POSTGRES_DB` и `POSTGRES_USER` в `.env` должны совпадать
с тем, что поднимает `db-up`: таска миграций использует их.

Фронтенд — ES-модули в `web/static/js/app/` и написанный руками
`web/static/css/app.css`. Шаблон подключает не исходники, а минифицированные
`app.min.js` / `app.min.css`; они в `.gitignore`. После правки JS или CSS
выполните `mise run assets`, а для пересборки при каждом сохранении держите
`mise run assets-watch` в отдельном терминале. Собирает esbuild (ставится
через `mise install`, npm-бэкенд): все модули в один ESM-файл, CSS в один
минифицированный файл.

### Проверка, что всё живо

```bash
curl -s localhost:8080/health                                 # {"success":true,...}
curl -s "localhost:8080/api/map-items" | head -c 200          # точки для карты
curl -s "localhost:8080/fragments/live-count"                 # счётчик «на карте сейчас»
```

Карта пустая — не прогнан `seed`. На карточках нет блока условий — не прогнан
`conditions_assess`. Погода не грузится — проверьте `WEATHER_MODE` (если
Open-Meteo недоступен, ставьте `mock`).

### Команды разработчика

```bash
mise run db-up              # только Postgres в Docker
mise run migrate-up         # накатить миграции; migrate-down — откатить
mise run seed               # демо-места
mise run weather-load       # погода (WEATHER_MODE=real|mock — в .env)
mise run conditions-assess  # оценки условий: DATE, ACTIVITY=walk,sup,hike,run, PLACE=<uuid>
mise run elevations-enrich  # высоты точек (OVERWRITE=1 — перезаписать)
mise run coverage-probe     # какие данные реально приходят: DATE, PLACE
mise run map-ensure         # собрать недостающие архивы карты
go test ./...
go vet ./...
go run github.com/a-h/templ/cmd/templ@v0.3.1118 generate   # пересобрать шаблоны
```

Те же команды напрямую, с флагами:

```bash
go run ./cmd/server weather_load [-day YYYY-MM-DD] [-place UUID]
go run ./cmd/server coverage_probe [-day YYYY-MM-DD] [-place UUID]
go run ./cmd/server elevations_enrich [-place UUID] [-overwrite]
go run ./cmd/server conditions_assess [-day YYYY-MM-DD] [-activity walk,sup,hike,run] [-place UUID]
go run ./cmd/server forecast_refresh [-day YYYY-MM-DD] [-activity ...]
go run ./cmd/server pins_cleanup [-grace_hours 24]   # удалить просроченные метки «Сейчас»
```

Остальные таски — `mise tasks`.

## Docker и деплой

### Файлы окружения

- `.env` — секреты и прикладные настройки (`TELEGRAM_CLIENT_ID`, `TELEGRAM_BOT_TOKEN`, `OPENMETEO_API_KEY`, `SENTRY_DSN` и т.п.). Compose передаёт этот файл в контейнеры через `env_file`. `DATABASE_URL` и `PORT` контейнеры получают из `compose.yaml`, дублировать их в `.env` не нужно.
- `deploy.env` — несекретные параметры образа, HTTP-порта, кредов базы и SSH-деплоя. Compose подставляет их в `compose.yaml`; SSH-настройки скрипта в контейнер не попадают.

Оба файла исключены из Git и из Docker build context. В репозитории лежат только `.env.example` и `deploy.env.example`.

```bash
cp deploy.env.example deploy.env        # заполнить параметры площадки
chmod 600 .env
```

### Деплой на Linux-сервер

Нужны Docker с buildx локально, Docker Engine и Compose plugin на сервере, SSH-доступ и право
пользователя `DEPLOY_USER` запускать Docker. Локально и на сервере выполните `docker login ghcr.io`.
Токен registry в `deploy.env` не записывайте. Для ARM-сервера поменяйте `BUILD_PLATFORM` на
`linux/arm64`.

Перед первым деплоем передайте секреты отдельно (пример для значений из `deploy.env.example`):

```bash
ssh deploy@server.example.com 'install -d -m 700 /opt/hermes'
scp .env deploy@server.example.com:/opt/hermes/.env
ssh deploy@server.example.com 'chmod 600 /opt/hermes/.env'
```

Затем заполните `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_PATH`, `HERMES_IMAGE` и другие параметры в `deploy.env`:

```bash
./scripts/deploy.sh
```

Скрипт собирает Linux-образ вместе с фронтенд-ассетами и публикует его в registry с тегом короткого
Git SHA (или `HERMES_TAG`). На сервер копируются только Compose и несекретный `deploy.env`. Дальше
скрипт делает `pull`, поднимает `db` и ждёт его healthy, накатывает миграции из `migrations/`,
поднимает остальное и ждёт healthy-статуса `api`. Секретный `.env` скрипт не копирует и не выводит.
`SKIP_BUILD=1` выкатывает уже опубликованный образ без сборки.

Офлайн-карты в образ не входят: соберите их (`mise run map-ensure`) и положите на сервер в
`/opt/hermes/web/static/pmtiles/`. Каталог примонтирован в контейнер только для чтения.
Фото пользователей хранятся в named volume `hermes_uploads`.

Частые операции на сервере:

```bash
cd /opt/hermes
docker compose --env-file deploy.env ps
docker compose --env-file deploy.env logs --tail=100 -f api
curl --fail http://127.0.0.1:8080/health

# Обновить Compose без пересборки того же тега (запускать локально, не на сервере):
SKIP_BUILD=1 ./scripts/deploy.sh

# Откат на ранее опубликованный образ (запускать локально):
HERMES_TAG=<previous_git_sha> SKIP_BUILD=1 ./scripts/deploy.sh

# Консольные команды (seed, user_create, events_import и т.д.):
docker compose --env-file deploy.env run --rm --entrypoint /app/hermes console seed
```

### Kubernetes

Helm-чарт лежит в `helm/hermes/`: приложение, PostGIS в StatefulSet, PVC для
архивов карты и загруженных фото, Ingress. Секреты в values не хранятся, их
берут из `secrets.existingSecret` или `externalSecrets`. Настройки площадки —
в `values.production.yaml`.

## Погода и оценка условий

Загрузка данных и их оценка разделены:

- `internal/infrastructure/openmeteo` — провайдер. Источники weather, marine и
  air quality запрашиваются, хранятся и обновляются отдельно (`FetchSource`):
  сбой одного не влияет на другие, и горизонт у них разный. Ряды склеиваются
  по UTC-таймстемпу, а не по индексу массива. `DemoProvider` включается только
  явно (`WEATHER_MODE=mock`); сбой реального API никогда не подменяется
  демо-данными.
- `internal/domain/scoring` — доменный scorer. Принимает `activity`
  (`walk`, `sup`, `hike`, `run`) и почасовое наблюдение, возвращает комфортный
  балл, отдельные предупреждения о рисках, объяснения (фактор → значение →
  вклад в штраф) и версию алгоритма (`heuristic-2.1.0`).
- `internal/application/usecase` — `LoadWeatherUseCase` только грузит погоду,
  `AssessConditionsUseCase` только считает оценки.

Загружается скользящее окно: последние 6 часов, сегодня и 9 дней вперёд.
`weather_load` и `conditions_assess` всегда обновляют окно целиком, поэтому
оценка не «залипает» на текущем моменте. Погода и море грузятся на 10 дней,
качество воздуха на 7: дальше upstream-API не отдаёт. Фактический горизонт
каждого источника клиент видит в поле `coverage`.

Инварианты:

- Отсутствующие значения хранятся и отдаются как `null`, не как `0`.
- Неизвестная высота волны не означает штиль: критичных данных нет, поэтому
  балл ограничен, уверенность `low`, вердикт не может быть `go`.
- Высокий комфортный балл не гарантирует безопасность: `verdict` и
  `warnings` считаются отдельно от `comfort_score`.
- Время хранится в UTC, календарный день считается в `Asia/Vladivostok`
  (сутки — с 14:00 UTC предыдущего дня до 14:00 UTC).

Сетка модели Open-Meteo около 11 км, поэтому без явной высоты горная точка
получает погоду города. Высота точки запрашивается у Open-Meteo, хранится вместе
с источником (`places.elevation_source`) и передаётся в запрос погоды. Что
реально приходит и для каких точек — в
[docs/weather-data-coverage.md](docs/weather-data-coverage.md).

## Карта

Векторная карта края, рельеф и горизонтали собираются в PMTiles-архивы и
отдаются сервером байтовыми диапазонами. Сценарий «Природа» показывает рельеф
с горизонталями, тропами и вершинами, остальные сценарии — городскую подложку.
Стиль, токены, офлайн-кеш и сборка архивов описаны в [MAP.md](MAP.md).

## API

Радар «Сейчас», каталог мест и событий, предложения пользователей, загрузка
фото и прогноз описаны в [docs/api.md](docs/api.md).

## Офлайн-стратегия

Сейчас офлайн работает только карта: куски архивов кешируются в IndexedDB,
а из профиля карту можно скачать целиком ([MAP.md](MAP.md#офлайн)).
Остальное — план, взятый из прототипа `vlru-places`, пока не реализованный:

- оболочка (HTML/CSS/JS) — precache, обновление при деплое;
- страницы `/pages/*` — cache-first после первого визита;
- фрагменты `/fragments/*` — network-first с fallback на кеш;
- погода и прогнозы — network-first, кеш в IndexedDB на 30 минут;
- планы поездок и репорты — IndexedDB + outbox, синхронизация по событию `online`;
- индикатор «не в сети, данные на HH:MM» в шапке.

## Документация

| Документ | О чём |
| :--- | :--- |
| [MAP.md](MAP.md) | Карта: стиль, режимы, офлайн, сборка архивов |
| [web/map/PIPELINE.md](web/map/PIPELINE.md) | Шаги сборки рельефа, горизонталей и воды |
| [docs/api.md](docs/api.md) | HTTP API |
| [docs/weather-data-coverage.md](docs/weather-data-coverage.md) | Что отдаёт Open-Meteo и для каких точек |
| [vlru-places/](vlru-places/README.md) | Дизайн-прототип и дизайн-система |
| [TODO.md](TODO.md) | Известные баги и отложенные риски |
