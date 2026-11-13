# hermes

## О сервисе

Приложение планировнаия активного отдыха ВЫХОДИ (рабочее название).

Ответственная команда: **PihPohPahPuh**

## Стек

- Backend: Go 1.27, PostgreSQL, PostGIS
- Frontend: htmx + Alpine.js (основа), React 18 + Vite (карта)
  - Карта: MapLibre GL JS + PMTiles (векторные тайлы, офлайн-архив региона)
  - Клиентский кеш: IndexedDB (Dexie.js), Cache Storage
  - Service Worker: Workbox (precache оболочки, runtime-cache фрагментов)

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

API: `GET /forecast/place/{id}?activity=sup&date=YYYY-MM-DD` возвращает
почасовые оценки, `sources` (источник, доступность, demo), `contributions`,
`warnings` и `limitations`.

## Офлайн-стратегия

- Оболочка (HTML/CSS/JS) - precache, обнова при деплое.
- Страницы `/pages/*` - cache-first после первого визита.
- Фрагменты `/fragments/*` - network-first, fallback на кэш.
- Погода/прогнозы - network-first, кэш в IndexedDB TTL 30m.
- Планы поездок и репорты - IndexedDB + outbox, синхронизация при online-событии.
- Статус: индикатор «не в сети, данные на HH:MM» в шапке.
