# hermes

## О сервисе

Приложение планировнаия активного отдыха ВЫХОДИ (рабочее название).

Ответственная команда: **PihPohPahPuh**

## Стек

- Backend: Go 1.27, PostgreSQL, PostGIS
- Frontend: серверный рендер templ + htmx + Alpine.js (основа, без React), картография — MapLibre GL JS + PMTiles (векторные тайлы, офлайн-архив региона)

## Локальный запуск

Ниже — полный цикл «с нуля до работающего сайта». Предполагается macOS/Linux, Docker для PostgreSQL.

### 0. Требования

- Go 1.27.3 (ставится через `mise install`, см. `mise.toml`)
- Docker — для PostgreSQL 15+ с PostGIS (или свой инстанс с уже созданными расширениями)
- `mise` — опционально, удобные алиасы (`mise run seed` и т.д.); все команды ниже даны и в виде `go run ...`

### 1. Окружение

```bash
mise run bootstrap        # создаёт .env из .env.example и качает модули
# или вручную: cp .env.example .env
```

Ключевые переменные (полный список — в `.env.example`):

| Переменная | По умолчанию | Что влияет |
| :--- | :--- | :--- |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/hermes?sslmode=disable` | подключение к БД |
| `PORT` | `8080` | порт HTTP-сервера |
| `WEB_DIR` | `./web/static` | корень статики (CSS/JS/загруженные фото) |
| `WEATHER_MODE` | `real` | `mock` — демо-данные без сети; `real` — запросы к Open-Meteo |
| `PHOTOS_STORAGE_DIR` | `<WEB_DIR>/uploads` | куда писать загруженные фото |
| `PHOTOS_MAX_UPLOAD_MB` | `10` | лимит одной загрузки фото |

### 2. Поднять PostgreSQL

```bash
docker run -d --name postgres-1 \
  -e POSTGRES_DB=hermes \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -p 5432:5432 \
  postgis/postgis:15-3.4
```

Имена контейнера/БД/пользователя должны совпадать с `POSTGRES_CONTAINER`, `POSTGRES_DB`, `POSTGRES_USER` в `.env` (таска миграций использует их). Со своим инстансом просто поправьте `DATABASE_URL`.

### 3. Применить миграции

Миграции — идемпотентные SQL-файлы в `migrations/` (`CREATE ... IF NOT EXISTS`), накатываются в любом порядке повторно:

```bash
mise run migrate-up
# или вручную, без mise/docker-exec:
psql "postgres://postgres:postgres@localhost:5432/hermes?sslmode=disable" \
  -v ON_ERROR_STOP=1 -f migrations/001_init.sql
```

Откат таблиц: `mise run migrate-down`.

### 4. Наполнить тестовыми данными

```bash
mise run seed        # 5 мест вокруг Владивостока (we смотровые, бухты, парк)
# = go run ./cmd/server seed
```

Сид создаёт только места. Погода и оценки условий считаются отдельными командами — так сид никогда не выдаёт фейковые «готовые» рекомендации:

```bash
# вариант А: без интернета к Open-Meteo — явные демо-данные
WEATHER_MODE=mock go run ./cmd/server weather_load
WEATHER_MODE=mock go run ./cmd/server conditions_assess

# вариант Б: реальный прогноз (нужен выход к api.open-meteo.com)
go run ./cmd/server weather_load            # скользящее окно: -6 ч + сегодня + 9 дней
go run ./cmd/server conditions_assess       # блок «Еду / Осторожно / Нет» на карточках мест
```

Дополнительно:

```bash
go run ./cmd/server elevations_enrich       # высоты точек (нужны для горной погоды)
NAME=Аня go run ./cmd/server user_create    # пользователь для входа (или просто войдите в UI по имени)
go run ./cmd/server pins_cleanup            # снять просроченные метки «Сейчас»
```

### 5. Запустить сервер

```bash
mise run dev                          # соберёт JS-бандл, дальше air с hot-reload, иначе go run
# или явно:
mise run assets                       # собрать и минифицировать JS (esbuild → app.min.js)
go run ./cmd/server listen            # аргумент listen можно опустить — он дефолтный
```

Фронтенд — ES-модули в `web/static/js/app/` и руками написанный `web/static/css/app.css`; шаблон подключает не исходники, а собранные минифицированные `app.min.js` / `app.min.css` (в `.gitignore`, артефакты сборки). После правки JS или CSS пересоберите их: `mise run assets`, а для постоянной пересборки при сохранении — `mise run assets-watch` (отдельный терминал). Сборка — esbuild (ставится через `mise install`, npm-бэкенд): 15 JS-модулей → один ESM-файл (~60 КБ), CSS 124 → 92 КБ.

Сайт: **http://localhost:8080** — карта с панелью, каталог, «Сейчас», добавление места/события, профиль. Вход — по имени, одним шагом, сессия в cookie. Полезные страницы: `/health` (liveness), `/catalog`, `/now`, `/rules`, `/profile`.

### 6. Проверка, что всё живо

```bash
curl -s localhost:8080/health                                 # {"success":true,...}
curl -s "localhost:8080/api/map-items" | head -c 200          # точки для карты
curl -s "localhost:8080/fragments/live-count"                 # счётчик «на карте сейчас»
```

Если карта пустая — не прогнан `seed`; если на карточках нет блока условий — не прогнан `conditions_assess`; если штормит погода — смотрите `WEATHER_MODE` (при недоступном Open-Meteo ставьте `mock`).

### Полезные команды разработчика

```bash
go test ./...                              # тесты
go run github.com/a-h/templ/cmd/templ@v0.3.1118 generate   # пересобрать шаблоны из web/templates/*.templ
mise run templ                             # то же через mise
go vet ./...                               # vet
```

Загруженные пользователями фото лежат в `web/static/uploads/` (путь задаётся `PHOTOS_STORAGE_DIR`) и раздаются сами — каталог добавлен в `.gitignore`, в репозиторий не коммитится.

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
- `primorsky-dem.pmtiles` — DEM в terrarium-RGB (z3–12): MapLibre строит из
  него объём (terrain) и драпирует зоны по высотам.

```bash
mise run map-relief                 # весь край
mise run map-relief-local           # только агломерация (быстро)
mise run map-serve                  # оба режима на :8080/map/ (переключатель «Город/Природа»)
```

На странице карта работает в двух режимах: **Город** — обычная векторная
карта, **Природа** — рельеф с высотными зонами (8–20 цветовых слоёв по
минимуму/максимуму DEM) и 3D-объёмом (камера наклоняется, terrain
экстраполирует высоты), плюс вода, границы, вершины и подписи. Режимы
взаимоисключающие: растровый рельеф не накладывается на векторную карту.
Без архива рельефа кнопка «Природа» выключена, «Город» работает как раньше;
без DEM-архива природа рисуется плоско.

Архивы отдаются как `GET /api/map/relief.pmtiles` и `GET /api/map/dem.pmtiles`
и подключаются в `web/map/style.json` источниками `relief` и `relief-dem`
(слой `relief-zones`, тег `hermes:mode: nature`). Подробности о том, какие
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

> Текущая реализация фронтенда (`web/`) — серверный рендер + htmx/Alpine: Service Worker и IndexedDB-кэш из списка ниже пока не подключены, это план по прототипу `vlru-places`.

- Оболочка (HTML/CSS/JS) - precache, обнова при деплое.
- Страницы `/pages/*` - cache-first после первого визита.
- Фрагменты `/fragments/*` - network-first, fallback на кэш.
- Погода/прогнозы - network-first, кэш в IndexedDB TTL 30m.
- Планы поездок и репорты - IndexedDB + outbox, синхронизация при online-событии.
- Статус: индикатор «не в сети, данные на HH:MM» в шапке.
