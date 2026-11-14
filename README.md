# hermes

## О сервисе

Приложение планировнаия активного отдыха ВЫХОДИ (рабочее название).

Ответственная команда: **PihPohPahPuh**

## Стек

- Backend: Go 1.27, PostgreSQL, PostGIS
- Frontend: серверный рендер templ + htmx + Alpine.js (основа, без React), картография — MapLibre GL JS + PMTiles (векторные тайлы, офлайн-архив региона)

## Локальный запуск

Одна команда — от нуля до работающего сайта:

```bash
mise run dev
```

Таски выстроены цепочкой (mise запускает независимые шаги параллельно):

1. `bootstrap` — `.env` из `.env.example`, `go mod download`;
2. `db-up` — PostGIS в Docker (`postgis/postgis:18-3.6`, контейнер
   `POSTGRES_CONTAINER`): работающий переиспользуется, остановленный
   запускается с прежними данными, иначе создаётся новый. Если на
   `POSTGRES_PORT` уже отвечает другая база, используется она;
3. `migrate-up` — `go run ./cmd/server migrate`: идемпотентные миграции из
   `migrations/` по `DATABASE_URL`, без psql и `docker exec`;
4. `data-load` — `seed` (5 мест вокруг Владивостока, повторный запуск
   обновляет, а не дублирует) + погода в `mock`-режиме + `conditions_assess`;
5. параллельно — `assets` (JS/CSS-бандлы esbuild) и `map-ensure` (архивы
   карты: если их нет, собирает `map-download` и `map-relief`, нужен Docker);
6. сервер `go run ./cmd/server listen`.

Сайт: **http://localhost:8080**. Вход — по имени, одним шагом, сессия в
cookie. Полезные страницы: `/health`, `/catalog`, `/now`, `/rules`, `/profile`.

Нужен Docker — или своя PostGIS: укажите её в `DATABASE_URL` и `POSTGRES_PORT`,
`db-up` увидит открытый порт и пропустит контейнер. Образы `postgis/postgis`
собираются только под amd64, на Apple Silicon контейнер идёт через эмуляцию
(`--platform linux/amd64`).

Офлайн-архивы карты (`primorsky-krai-z15.pmtiles`, `primorsky-relief.pmtiles`)
в репозиторий не входят; `map-ensure` собирает недостающие при первом запуске
(векторный — 5–10 мин, рельеф — дольше: качается DEM всего края). Без
векторного архива подложки нет («Карта не загрузилась»), без рельефа «Природа»
рисуется городской подложкой. Подробности — в [MAP.md](MAP.md).

Ключевые переменные (полный список — в `.env.example`):

| Переменная | По умолчанию | Что влияет |
| :--- | :--- | :--- |
| `DATABASE_URL` | `postgres://postgres:***@localhost:5432/hermes?sslmode=disable` | подключение к БД |
| `PORT` | `8080` | порт HTTP-сервера |
| `WEB_DIR` | `./web/static` | корень статики (CSS/JS/загруженные фото) |
| `WEATHER_MODE` | `real` | `mock` — демо-данные без сети; `real` — запросы к Open-Meteo |
| `PHOTOS_STORAGE_DIR` | `<WEB_DIR>/uploads` | куда писать загруженные фото |
| `PHOTOS_MAX_UPLOAD_MB` | `10` | лимит одной загрузки фото |

Имена контейнера/БД/пользователя (`POSTGRES_CONTAINER`, `POSTGRES_DB`,
`POSTGRES_USER` в `.env`) должны совпадать с тем, что поднимает `db-up` —
таска миграций использует их.

Фронтенд — ES-модули в `web/static/js/app/` и руками написанный
`web/static/css/app.css`; шаблон подключает не исходники, а собранные
минифицированные `app.min.js` / `app.min.css` (в `.gitignore`, артефакты
сборки). После правки JS или CSS пересоберите их: `mise run assets`, а для
постоянной пересборки при сохранении — `mise run assets-watch` (отдельный
терминал). Сборка — esbuild (ставится через `mise install`, npm-бэкенд):
19 JS-модулей → один ESM-файл (~80 КБ), CSS 124 → 92 КБ.

### Проверка, что всё живо

```bash
curl -s localhost:8080/health                                 # {"success":true,...}
curl -s "localhost:8080/api/map-items" | head -c 200          # точки для карты
curl -s "localhost:8080/fragments/live-count"                 # счётчик «на карте сейчас»
```

Если карта пустая — не прогнан `seed`; если на карточках нет блока условий —
не прогнан `conditions_assess`; если штормит погода — смотрите `WEATHER_MODE`
(при недоступном Open-Meteo ставьте `mock`).

### Полезные команды разработчика

```bash
mise run db-up            # только Postgres в Docker
mise run map-ensure       # собрать недостающие архивы карты
mise run migrate-up       # накатить миграции (go run ./cmd/server migrate); migrate-down — откат таблиц
mise run seed             # места
mise run weather-load     # реальная погода (WEATHER_MODE=real|mock — в .env)
mise run conditions-assess
go test ./...             # тесты
go vet ./...
go run github.com/a-h/templ/cmd/templ@v0.3.1118 generate   # пересобрать шаблоны
```

Перечень остальных тасок — `mise tasks`.

## Docker и деплой

### Файлы окружения

- `.env` — секреты и прикладные настройки (`TELEGRAM_CLIENT_ID`, `TELEGRAM_BOT_TOKEN`, `OPENMETEO_API_KEY`, `SENTRY_DSN` и т.п.). Compose передаёт этот файл в контейнеры через `env_file`; `DATABASE_URL` и `PORT` контейнеры получают из `compose.yaml`, дублировать их в `.env` не нужно.
- `deploy.env` — не секретные параметры образа, HTTP-порта, кредов базы и SSH-деплоя. Эти значения используются для подстановок Compose; SSH-настройки скрипта в контейнер не передаются.

Оба файла игнорируются Git и Docker build context. В репозитории хранятся только `.env.example` и `deploy.env.example`.

```bash
cp deploy.env.example deploy.env        # заполнить параметры площадки
chmod 600 .env
```

### Деплой на Linux-сервер

Нужны Docker с buildx локально, Docker Engine и Compose plugin на сервере, SSH-доступ и право
пользователя `DEPLOY_USER` запускать Docker. Локально и на сервере выполнить `docker login ghcr.io`.
Токен registry в `deploy.env` не записывать. Для ARM-сервера изменить `BUILD_PLATFORM` на
`linux/arm64`.

Перед первым деплоем отдельно передать секреты (пример для значений из `deploy.env.example`):

```bash
ssh deploy@server.example.com 'install -d -m 700 /opt/hermes'
scp .env deploy@server.example.com:/opt/hermes/.env
ssh deploy@server.example.com 'chmod 600 /opt/hermes/.env'
```

Затем заполнить `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_PATH`, `HERMES_IMAGE` и другие параметры в `deploy.env`:

```bash
./scripts/deploy.sh
```

Скрипт собирает Linux-образ (включая фронтенд-ассеты), публикует его в registry с тегом короткого
Git SHA (или `HERMES_TAG`), копирует на сервер только Compose и не секретный `deploy.env`, выполняет
`pull`, поднимает `db`, ждёт его healthy, накатывает идемпотентные миграции из `migrations/`, поднимает
остальное и ждёт healthy-статуса `api`. Секретный `.env` скрипт не копирует и не выводит. Для повторного
выката уже опубликованного образа без сборки используется `SKIP_BUILD=1`.

Офлайн-карты в образ не входят: соберите их (`mise run map-download`, `mise run map-relief`) и
положите на сервер в `/opt/hermes/web/static/pmtiles/` — каталог примонтирован в контейнер read-only.
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

## Погода и оценка условий

Данные и их оценка разделены:

- `internal/infrastructure/openmeteo` — провайдер. Каждый источник
  (**weather**, **marine**, **air quality**) запрашивается, хранится и
  обновляется отдельно: `FetchSource`. Сбой одного не влияет на другие, а
  горизонт у них разный. Ряды склеиваются **по UTC-таймстемпу**, а не по
  индексу массива. `DemoProvider` — явный demo-режим
  (`WEATHER_MODE=mock`); сбой реального API никогда не подменяется
  демо-данными.
- `internal/domain/scoring` — доменный scorer. Принимает `activity`
  (`walk`, `sup`, `hike`, `run`) и почасовое наблюдение, возвращает комфортный
  балл, отдельные предупреждения о рисках, объяснения (фактор → значение →
  вклад в штраф) и версию алгоритма (`heuristic-2.1.0`).
- `internal/application/usecase` — `LoadWeatherUseCase` только грузит погоду,
  `AssessConditionsUseCase` только считает оценки.

### Горизонт: текущая погода + 10 дней

Загружается не один день, а **скользящее окно**: последние 6 часов + сегодня
+ 9 дней вперёд. `weather_load` и `conditions_assess` всегда обновляют окно
целиком, поэтому оценка не «залипает» на текущем моменте.

Горизонты источников различаются и это видно клиенту в `coverage`:

| Источник | Горизонт | Почему |
| :--- | :--- | :--- |
| weather | 10 дней | полный прогноз Open-Meteo |
| marine | 10 дней | тот же горизонт |
| air quality | **7 дней** | жёсткий лимит upstream-API |

Короткий горизонт AQI не урезает остальные источники: погода и море
по-прежнему грузятся на все 10 дней.

API: `GET /forecast/place/{id}?activity=sup&days=10` вернёт до 10 дней;
`days` по умолчанию `1` (только сегодня).

Ключевые инварианты:

- Отсутствующие значения хранятся и отдаются как `null`, а не `0`.
- Неизвестная высота волны — не «штиль»: критичных данных нет → балл
  ограничен, уверенность `low`, вердикт не может быть `go`.
- Высокий комфортный балл не гарантирует безопасность: `verdict` и
  `warnings` живут отдельно от `comfort_score`.
- Время хранится в UTC, календарный день трактуется в `Asia/Vladivostok`
  (сутки = 14:00 UTC предыдущего дня → 14:00 UTC).

### Высоты и рельеф

Высота точки нужна и для оценки похода, и для корректной погоды: модель
Open-Meteo имеет сетку ~11 км, поэтому без явной высоты горная точка
получает погоду города. Высота запрашивается у Open-Meteo, хранится с
провенансом (`places.elevation_source`) и подставляется в запрос погоды.

```bash
mise run elevations-enrich          # заполнить высоты точек (OVERWRITE=1 — перезаписать)
mise run coverage-probe             # что реально приходит и какие точки делят одну ячейку
```

Рельеф на карте — два растровых архива из Copernicus DEM GLO-90, потому что
векторные тайлы OSM высот не содержат:

- `primorsky-relief.pmtiles` — дискретные высотные зоны с запечённой отмывкой
  (z3–13, PNG, ресемплинг nearest: границы зон читаются «лесенкой»);
- `primorsky-dem.pmtiles` — DEM в terrarium-RGB (z3–12); приложение его не
  использует: рельеф плоский, объём даёт отмывка.

```bash
mise run map-relief                 # весь край
mise run map-relief-local           # только агломерация (быстро)
mise run map-serve                  # отладка стиля на :8080/map/ (Город/Природа/Море)
```

Сценарий «Природа» включает режим `nature`: рельеф с высотными зонами
(8–20 цветовых ступеней по минимуму/максимуму DEM) плюс вода, тропы, границы,
вершины и подписи; остальные сценарии — городскую подложку. Режимы
взаимоисключающие: растровый рельеф не накладывается на векторную карту.
Стиль, токены и офлайн — в [MAP.md](MAP.md). Подробности о том, какие
данные реально получаются и для каких точек — в
[docs/weather-data-coverage.md](docs/weather-data-coverage.md).

### Консольные команды

```bash
mise run seed                       # только места
mise run weather-load               # + параметры ниже
mise run conditions-assess          # только оценка (эвристики)

go run ./cmd/server weather_load [-day YYYY-MM-DD] [-place UUID]
go run ./cmd/server coverage_probe [-day YYYY-MM-DD] [-place UUID]
go run ./cmd/server elevations_enrich [-place UUID] [-overwrite]
go run ./cmd/server conditions_assess [-day YYYY-MM-DD] [-activity walk,sup,hike,run] [-place UUID]
go run ./cmd/server forecast_refresh [-day YYYY-MM-DD] [-activity ...]
go run ./cmd/server pins_cleanup [-grace_hours 24]   # удалить просроченные метки «Сейчас»
WEATHER_MODE=mock go run ./cmd/server weather_load   # явный demo-режим
```

## Радар «Сейчас» и каталог

Радар — быстрые временные метки жителей (встречи, спорт, помощь). Метка живёт
от `starts_at` до `ends_at`, автор идёт по определению (`going_total = count(pin_goings) + 1`).
Мутации требуют сессии; чтение доступно анонимно и декорируется для своих
(`joined`, `mine`, `going_total`).

- `GET /pins?bbox=minLon,minLat,maxLon,maxLat&kind=sport` — активные метки во вьюпорте
- `GET /pins/{id}` · `POST /pins` · `DELETE /pins/{id}` (автор)
- `PUT /pins/{id}/going` / `DELETE` — «Я приду»
- `POST /pins/{id}/reports` — жалоба; при `PINS_REPORT_HIDE_THRESHOLD` жалобах метка скрывается

Модерация задаётся окружением (по умолчанию: 5 живых меток на пользователя,
10 в час, скрытие после 3 жалоб, максимум 12 ч жизни метки):

| Переменная | Значение по умолчанию |
| :--- | :--- |
| `PINS_MAX_ACTIVE_PER_USER` | `5` (0 — без лимита) |
| `PINS_MAX_PER_HOUR` | `10` (0 — без лимита) |
| `PINS_STOP_WORDS` | пусто, список через запятую |
| `PINS_NIGHT_BAN_FROM` / `PINS_NIGHT_BAN_TO` | пусто (выкл), локальное время `HH:MM` |
| `PINS_REPORT_HIDE_THRESHOLD` | `3` (0 — выкл) |
| `PINS_MAX_DURATION_H` | `12` |
| `PINS_PURGE_GRACE_H` | `24`, используется `pins_cleanup` |

Каталог мест расширен карточкой из дизайна (категория по сценарию, район, адрес,
часы, цена, сезон, доступ, теги, фото с атрибуцией, тип берега). Место и событие
адресуются UUID или slug.

- `GET /places?q=&category=&layer=&type=&lat=&lon=&radius_m=` — поиск по названию, описанию, адресу, району и тегам
- `GET /places/{id}` (UUID или slug) · `GET /places/{id}/similar?limit=6`
- `GET /places/stats?scenario=sea` — агрегаты вкладок: `category_counts`, `type_counts`
- `GET /events?place_id=&category=&q=&all=1` — предстоящие по умолчанию, `all=1` включает прошедшие
- `GET /events/{id}` (UUID или slug)
- `GET /collections?scenario=` · `GET /collections/{id}` — редакционные подборки с элементами

## Предложения и фото

Пользовательские предложения (новые места, события, правки) проходят редакционный
цикл `review → published/rejected`. Автор видит только свои. Повторная отправка с
тем же заголовком `Idempotency-Key` и телом возвращает сохранённую запись (201 → 200),
с другим телом — 409. Тип `fix` требует `fix_id` и `fix_reasons`, `event` — `date`.

- `POST /submissions` · `GET /submissions?status=review` · `GET /submissions/{id}`
- `POST /photos` — загрузка фото (multipart, поле `file`, лимит `PHOTOS_MAX_UPLOAD_MB`, по умолчанию 10 МБ; JPG/PNG/WebP). Файл сразу сохраняется на диск (`PHOTOS_STORAGE_DIR`, по умолчанию `<WEB_DIR>/uploads/photos/гггг-мм/{id}.jpg`), строка падает в `photos` со статусом `active`: id — UUIDv7 (совпадает с именем файла), фиксируются размеры и `sha256`, автор — из сессии. Хранятся два JPEG-варианта: полный (сторона ≤ 1600 px) и миниатюра (≤ 640 px). Ответ — `201 {id, status, url, url_sm, width, height}`; ошибки: 400 не-изображение, 401 без входа, 403 без CSRF, 413 сверх лимита. Фото прицепляются к заявке полем `photos[]` (`src`, `src_sm`, `width`, `height`) и показываются в профиле миниатюрой.

API: `GET /forecast/place/{id}?activity=sup&date=YYYY-MM-DD` возвращает
почасовые оценки, `sources` (источник, доступность, demo), `contributions`,
`warnings` и `limitations`.

## Офлайн-стратегия

> Текущая реализация фронтенда (`web/`) — серверный рендер + htmx/Alpine. Из списка ниже работает только карта: куски архивов кешируются в IndexedDB, а из профиля карту можно скачать целиком ([MAP.md](MAP.md#офлайн)). Service Worker и остальные пункты — план по прототипу `vlru-places`.

- Оболочка (HTML/CSS/JS) - precache, обнова при деплое.
- Страницы `/pages/*` - cache-first после первого визита.
- Фрагменты `/fragments/*` - network-first, fallback на кэш.
- Погода/прогнозы - network-first, кэш в IndexedDB TTL 30m.
- Планы поездок и репорты - IndexedDB + outbox, синхронизация при online-событии.
- Статус: индикатор «не в сети, данные на HH:MM» в шапке.

## TODO

- **Нет Service Worker для оболочки.** Без сети не открывается сама страница, поэтому скачанная карта помогает только на уже открытой вкладке или когда недоступен сервер карты. Отложено: оболочка рендерится сервером с данными из БД, нужна общая офлайн-стратегия (см. выше). Вернуться, когда дойдём до офлайна приложения целиком.
- **Рельеф собран только для агломерации (z10–13).** На стартовом виде «Природы» (z7,4) его не видно. Нужна полная сборка `mise run map-relief` на машине с docker; вернуться перед демонстрацией сценария «Природа».
