# Пайплайн карты: план декомпозиции скриптов

Состояние на 2026-11-16. Два сборщика: городская карта (`build-map.sh`)
и рельеф (`build-relief.sh`). Проблема текущего вида: Python-скрипты
встроены heredoc'ами прямо в sh, шаги перемешаны с логикой оркестрации,
читать файл целиком, чтобы понять порядок действий, нельзя.

## Принципы

1. **Один шаг = один файл.** Тяжёлые шаги (Python, долгие gdal) — отдельные
   файлы рядом, не heredoc'и внутри sh.
2. **Оркестраторы — тонкие.** Только: проверка артефакта предыдущего шага →
   вызов шага → сообщение. Вся логика — в шагах.
3. **Каждый шаг идемпотентен.** Прогнал повторно — получил тот же результат,
   перезаписав выход. Маркер `*.done` — опционально, для долгих шагов с
   кешируемым результатом (мозаика DEM, скачивание).
4. **Комментарий в начале каждого файла-шага**: что на входе, что на выходе,
   одна строка «зачем».
5. **Именование шагов**: номер + глагол: `10-contours.sh`, `20-contours-mvt.sh`.

## Целевая структура scripts/map/

```
scripts/map/
├── build-map.sh                # оркестратор: городская карта (5 вызовов)
├── build-relief.sh             # оркестратор: рельеф (9 вызовов, см. ниже)
├── lib/
│   ├── common.sh               # log/fail, пути, docker_run хелпер
│   └── pmtiles.sh              # tiles→mbtiles→pmtiles, tilejson
├── map/                        # шаги городской карты (по мере правок)
├── relief/
│   ├── 10-dem-mosaic.sh        # скачивание GLO-30 + мозаика → dem/krai_dem.tif
│   ├── 20-smooth.py            # гауссово сглаживание DEM (сепарабельный фильтр)
│   ├── 15-land-cutline.sh      # контур суши: граница минус вода OSM → boundary-land.geojson
│   ├── 30-contours.sh          # gdal_contour 20 м / 100 м → GPKG
│   ├── 40-chaikin.py          # сглаживание линий Чайкином → *_smooth.gpkg
│   ├── 50-contours-merge.sh    # слияние GPKG + атрибут kind → contours_all.gpkg
│   ├── 52-contours-clip.sh     # обрезка контуров по границе края → contours_clip.gpkg
│   ├── 60-relief-raster.sh     # сетка x2 → hillshade → color-relief
│   ├── 70-blend.py            # цвет × отмывка → shaded_rgb.tif
│   ├── 80-mask.sh             # обрезка по контуру суши (-cutline -dstalpha)
│   ├── 90-relief-tiles.sh     # gdalwarp+gdal2tiles → тайлы z3-15
│   └── 95-contours-mvt.sh     # contours_clip.gpkg → MVT: ОТДЕЛЬНЫЙ ПРОГОН НА КАЖДЫЙ ЗУМ
│   │                          # (z5-11 major, z12, z13, z14 — все линии).
│   │                          # За один прогон 12-14 не влезает в RAM VM.
```

Что уходит из sh в файлы при декомпозиции:

- `chaikin.py`, `blend.py`, `smooth.py`, `zones.py` — уже существуют как
  файлы в `.cache/.../relief/` (скрипт пишет их heredoc'ом при каждом
  прогоне). Переносим в `scripts/map/relief/` под контролем git, heredoc'и
  удаляются.
- Логика `tiles_to_mbtiles` / tilejson из `build-relief.sh` →
  `lib/pmtiles.sh`.
- Каждый вызов `docker run` в шаге — через хелпер `docker_run` из
  `lib/common.sh` (единый образ, единый монт `/work`).

## Оркестратор build-relief.sh после декомпозиции

Весь файл — последовательность вида:

```bash
# оркестратор — тонкий: каждый шаг — файл из relief/, лог с метками времени
step "1/12 Мозаика DEM"             && bash relief/10-dem-mosaic.sh
step "2/12 Сглаживание DEM"         && run_py relief/20-smooth.py
step "2.5/12 Контур суши"           && bash relief/15-land-cutline.sh
step "3/12 Контуры 20/100 м"        && bash relief/30-contours.sh
step "4/12 Чайкин"                  && bash relief/40-chaikin-run.sh
step "5/12 Слияние контуров"        && bash relief/50-contours-merge.sh
step "5.5/12 Обрезка контуров"      && bash relief/52-contours-clip.sh
step "6/12 Отмывка и цвет"          && bash relief/60-relief-raster.sh
step "7/12 Смешивание"              && run_py relief/70-blend.py
step "8/12 Обрезка по границе"      && bash relief/80-mask.sh
step "9/12 Тайлы рельефа"           && bash relief/90-relief-tiles.sh
step "10/12 Упаковка relief"        && pack relief/tiles -> relief.pmtiles
step "11/12 MVT контуров (по зумам)" && bash relief/95-contours-mvt.sh
step "12/12 Упаковка contours"      && pack contours_mvt -> contours.pmtiles
# +13–14: terrarium DEM (RELIEF_SKIP_DEM=1 — пропустить)
```

Шаг 15 (контур суши) важен для береговой линии: административная граница
края рисует берег грубо и задирается на акваторию заливов, поэтому маска
шага 80 режет рельеф по `boundary-land.geojson` = граница минус полигоны
воды OSM (точный берег с островами). Контур пересобирать только при
смене границы или береговых данных.

## Структура web/map/ (всё для работы с картой — в одном месте)

```
web/map/
├── style.json          # стиль подложки: слои, режимы (hermes:mode), токены var(--map-*)
├── favicon.svg
├── vendor/             # MapLibre GL JS, PMTiles — без CDN
│   ├── maplibre-gl.js
│   ├── maplibre-gl.css
│   └── pmtiles.js
├── fonts/              # глифы Noto Sans (Regular/Bold/Italic)
└── PIPELINE.md         # этот документ: пайплайн сборки архивов
```

Вода собирается отдельно (`scripts/map/water/10-water-tiles.sh`,
`mise run map-water`): Natural Earth 50m ocean+lakes (шейпы кладутся в
`.cache/map/water/`) → MVT z0-10 по bbox с запасом вокруг края. К океану
добавляется заплатка «всё восточнее 139.31E — море»: из дыры океанского
полигона иначе проступал Хоккайдо. Слой `water-global` в стиле рисуется
под детальной водой края и только до z10 (`maxzoom: 10`) — выше акваторию
с островами рисует детальная вода OSM из архива края.

Клиентские модули карты — в `web/static/js/app/` (`map.js`, `map-style.js`,
`map-tiles.js`, `offline-map.js`), токены цветов — блок «Карта» в
`web/static/css/app.css`. Отладочная страница — `cmd/mapserver`
(`mise run map-serve` → `/map/?s=nature`).

Итоговые архивы (`web/static/pmtiles/`):

| Архив | Что | Zoom |
| --- | --- | --- |
| `primorsky-krai-z15.pmtiles` | городская карта из OSM (MVT) | 0–15 |
| `primorsky-relief.pmtiles` | зоны высот + отмывка (PNG, RGBA) | 3–15 |
| `primorsky-contours.pmtiles` | горизонтали (MVT, слой `contours`, поля `ELEV`, `kind`) | 5–14 |
| `primorsky-water.pmtiles` | мировая вода: океаны + озёра Natural Earth 50m (MVT, слой `water`) | 0–15 |
| `primorsky-dem.pmtiles` | DEM в terrarium-RGB (про запас, не используется) | 3–12 |

## Порядок внедрения

1. Вынести существующие heredoc-Python в `scripts/map/relief/` как файлы,
   sh начинает их вызывать — поведение не меняется.
2. Разрезать `build-relief.sh` на шаги 10–95 (механический перенос блоков).
3. То же для `build-map.sh` (там проще: 6 шагов).
4. Прогнать полную сборку обоими оркестраторами, сверить архивы байт-в-байт
   с текущей сборкой (или по checksum тайлов).
