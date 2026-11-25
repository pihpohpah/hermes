# Пайплайн сборки архивов карты

Три сборщика: векторная карта (`scripts/map/build-map.sh`), рельеф
(`scripts/map/build-relief.sh`) и мировая вода
(`scripts/map/water/10-water-tiles.sh`). Как карта устроена в приложении и
как запускать сборку — в [MAP.md](../../MAP.md).

## Правила для шагов

1. Один шаг — один файл. Python и долгие вызовы GDAL лежат в отдельных
   файлах, не heredoc'ами внутри sh.
2. Оркестратор только вызывает шаги по порядку и пишет лог с метками времени
   и длительностью каждого шага. Логика живёт в шагах.
3. Шаг идемпотентен: повторный прогон перезаписывает выход тем же
   результатом. Долгие шаги с кешируемым результатом (мозаика DEM,
   скачивание) пропускаются, если результат уже есть.
4. В начале файла-шага — комментарий: что на входе, что на выходе и зачем
   шаг нужен.
5. Имя шага — номер и действие: `30-contours.sh`, `95-contours-mvt.sh`.
   Каждый шаг делает `source lib/common.sh`, поэтому его можно запустить
   отдельно.

## Структура scripts/map/

```
scripts/map/
├── build-map.sh                # оркестратор векторной карты; `sources` — только скачать исходники
├── build-relief.sh             # оркестратор рельефа; `sources` — только собрать мозаику DEM
├── config.json, process.lua    # профиль tilemaker
├── Dockerfile.tools            # образ tilemaker + osmium
├── lib/
│   ├── common.sh               # пути, log/fail, docker-хелперы
│   ├── pmtiles.sh              # тайлы → MBTiles → PMTiles → verify, tilejson горизонталей
│   ├── mbtiles.py              # каталог {z}/{x}/{y} → MBTiles
│   └── dem-download.sh         # докачка одного DEM-тайла
├── relief/
│   ├── 10-dem-mosaic.sh        # скачивание GLO-30 + мозаика → dem/krai_dem.tif
│   ├── 15-land-cutline.sh      # контур суши: граница минус вода OSM → boundary-land.geojson
│   ├── 20-smooth.py            # гауссово сглаживание DEM (сепарабельный фильтр)
│   ├── 30-contours.sh          # gdal_contour 20 м / 100 м → GPKG
│   ├── 40-chaikin-run.sh       # запуск 40-chaikin.py для мелких и крупных горизонталей
│   ├── 40-chaikin.py           # сглаживание линий Чайкином → *_smooth.gpkg
│   ├── 50-contours-merge.sh    # слияние GPKG + атрибут kind
│   ├── 52-contours-clip.sh     # обрезка горизонталей по границе края
│   ├── 60-relief-raster.sh     # сетка x2 → hillshade → color-relief (палитра — zones.py)
│   ├── zones.py                # плавный высотный рамп по min/max DEM
│   ├── 70-blend.py             # цвет × отмывка → shaded_rgb.tif
│   ├── 80-mask.sh              # обрезка по контуру суши (-cutline -dstalpha)
│   ├── 90-relief-tiles.sh      # gdalwarp + gdal2tiles → PNG-тайлы z3–15
│   ├── 95-contours-mvt.sh      # горизонтали → MVT z5–14
│   ├── 96-dem-terrarium.sh     # DEM → terrarium-тайлы z3–12
│   └── terrarium.py            # кодирование высоты в terrarium-RGB
└── water/
    └── 10-water-tiles.sh       # Natural Earth ocean + lakes → MVT z0–10
```

Номер в имени файла задаёт место шага в пайплайне, оркестратор нумерует шаги
в логе по-своему (1–14).

## Рельеф

`build-relief.sh` выполняет шаги в таком порядке:

| Лог | Файл | Что |
| --- | --- | --- |
| 1 | `10-dem-mosaic.sh` | мозаика DEM; пропускается, если уже собрана для этого bbox (`RELIEF_REBUILD_DEM=1` — пересобрать) |
| 2 | `20-smooth.py` | сглаживание DEM |
| 2.5 | `15-land-cutline.sh` | контур суши |
| 3 | `30-contours.sh` | горизонтали 20/100 м |
| 4 | `40-chaikin-run.sh` | сглаживание Чайкином |
| 5 | `50-contours-merge.sh` | слияние + `kind` |
| 5.5 | `52-contours-clip.sh` | обрезка горизонталей |
| 6 | `60-relief-raster.sh` | отмывка и цвет на сетке x2 |
| 7 | `70-blend.py` | смешивание |
| 8 | `80-mask.sh` | обрезка по контуру суши |
| 9 | `90-relief-tiles.sh` | PNG-тайлы рельефа |
| 10 | `lib/pmtiles.sh` | упаковка `primorsky-relief.pmtiles` |
| 11 | `95-contours-mvt.sh` | MVT горизонталей |
| 12 | `lib/pmtiles.sh` | упаковка `primorsky-contours.pmtiles` |
| 13–14 | `96-dem-terrarium.sh`, `lib/pmtiles.sh` | terrarium-DEM и `primorsky-dem.pmtiles`; `RELIEF_SKIP_DEM=1` пропускает |

Контур суши нужен для береговой линии. Административная граница края рисует
берег грубо и заходит на акваторию заливов, поэтому шаг 80 режет рельеф по
`boundary-land.geojson`: граница минус полигоны воды OSM, точный берег с
островами. Без этого файла рельеф режется по самой границе, а
`map-relief-local` не режется вовсе. Пересобирать контур нужно только при
смене границы или береговых данных.

`95-contours-mvt.sh` нарезает z5–11 (только крупные горизонтали) одним
прогоном, а z12, z13 и z14 (все линии) — отдельным прогоном на каждый зум:
за один прогон ogr2ogr не укладывается в память VM.

Переменные окружения: `RELIEF_BBOX`, `RELIEF_MINZOOM`/`RELIEF_MAXZOOM`,
`DEM_MAXZOOM`, `RELIEF_OUTPUT`, `RELIEF_CONTOURS_OUTPUT`, `DEM_OUTPUT`,
`RELIEF_REBUILD_DEM=1`, `RELIEF_SKIP_DEM=1`, `RELIEF_BOUNDARY`,
`MAP_CACHE_DIR`.

## Вода

`mise run map-water` собирает `primorsky-water.pmtiles` из Natural Earth 50m
(ocean + lakes) в MVT z0–10, с обрезкой по bbox с запасом вокруг края. Шейпы
скрипт не скачивает: `ne_50m_ocean.zip` и `ne_50m_lakes.zip` нужно взять с
naturalearthdata.com и распаковать в `.cache/map/water/`.

К океану добавляется заплатка «всё восточнее 139.31E — море», иначе из дыры
океанского полигона проступает Хоккайдо. Слой `water-global` в стиле
рисуется под детальной водой края и только до z10 (`maxzoom: 10`); выше
акваторию с островами рисует вода OSM из архива края.

## web/map/

```
web/map/
├── style.json          # стиль подложки: слои, режимы (hermes:mode), токены var(--map-*)
├── favicon.svg
├── vendor/             # MapLibre GL JS, PMTiles, без CDN
│   ├── maplibre-gl.js
│   ├── maplibre-gl.css
│   ├── pmtiles.js
│   └── blank.html
├── fonts/              # глифы Noto Sans (Regular/Bold/Italic)
└── PIPELINE.md
```

Клиентские модули карты лежат в `web/static/js/app/` (`map.js`,
`map-style.js`, `map-tiles.js`, `offline-map.js`), токены цветов — в блоке
«Карта» в `web/static/css/app.css`. Отладочная страница — `cmd/mapserver`
(`mise run map-serve` → `/map/?s=nature`).

## Итоговые архивы

Все архивы складываются в `web/static/pmtiles/`.

| Архив | Что | Zoom |
| --- | --- | --- |
| `primorsky-krai-z15.pmtiles` | городская карта из OSM (MVT) | 0–15 |
| `primorsky-relief.pmtiles` | зоны высот + отмывка (PNG, RGBA) | 3–15 |
| `primorsky-contours.pmtiles` | горизонтали (MVT, слой `contours`, поля `ELEV`, `kind`) | 5–14 |
| `primorsky-water.pmtiles` | мировая вода: океаны + озёра Natural Earth 50m (MVT, слой `water`) | 0–10 |
| `primorsky-dem.pmtiles` | DEM в terrarium-RGB (приложение не использует) | 3–12 |
