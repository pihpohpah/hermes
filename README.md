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

Рельеф на карте — отдельный растровый архив отмывки (hillshade) из
Copernicus DEM GLO-90, потому что векторные тайлы OSM высот не содержат:

```bash
mise run map-relief                 # весь край, z8-13
mise run map-relief-local           # только агломерация (быстро)
mise run map-serve                  # оба режима на :8080/map/ (переключатель «Город/Природа»)
```

На странице карта работает в двух режимах: **Город** — обычная векторная
карта, **Природа** — рельеф с высотными зонами (8–20 цветовых слоёв по
минимуму/максимуму DEM) плюс вода, границы, вершины и подписи. Режимы
взаимоисключающие: растровый рельеф не накладывается на векторную карту.
Без архива рельефа кнопка «Природа» выключена, «Город» работает как раньше.

Рельеф отдаётся как `GET /api/map/relief.pmtiles` и подключается в
`web/map/style.json` слоем `relief-zones` (тег `hermes:mode: nature`).
Подробности о том, какие данные реально получаются и для каких точек — в
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
WEATHER_MODE=mock go run ./cmd/server weather_load   # явный demo-режим
```

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
