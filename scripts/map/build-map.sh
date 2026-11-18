#!/usr/bin/env bash
# Собирает офлайн-архив карты Приморского края (PMTiles) из OSM-экстракта.
#
# Пайплайн (см. MAP.md, раздел «Как перегенерировать карту»):
#   1. скачать экстракт Дальневосточного ФО с Geofabrik (кэшируется в .cache/map)
#   2. достать из экстракта relation административной границы (по умолчанию
#      151225 — Приморский край) и конвертировать его в GeoJSON-полигон
#   3. вырезать регион из экстракта по полигону границы:
#      прямоугольный bbox тянул бы Китай, Хабаровский край и ЕАО
#   4. обрезать береговую линию (Water polygons) по той же границе через GDAL
#   5. отрисовать векторные тайлы tilemaker'ом (профиль OpenMapTiles + правки
#      под хайкинг: тропы с z12, атрибут hiking_route=1)
#   6. конвертировать MBTiles -> PMTiles и проверить архив (pmtiles verify)
#
# Внешние зависимости: docker + curl + unzip (образы подтягиваются автоматически).
#
# Переопределяемые переменные:
#   MAP_MAXZOOM            верхний зум (по умолчанию 15; при нестандартном
#                          значении поправьте MAX_ZOOM в process.lua)
#   MAP_OUTPUT             куда положить архив
#   MAP_KRAI_RELATION_ID   граница региона (151225 — Приморский край,
#                          1647394 — Владивостокский городской округ)
#   MAP_CACHE_DIR          кэш промежуточных файлов
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PROFILE_DIR="$ROOT_DIR/scripts/map"
KRAI_RELATION_ID="${MAP_KRAI_RELATION_ID:-151225}"
MAXZOOM="${MAP_MAXZOOM:-15}"
OUTPUT="${MAP_OUTPUT:-$ROOT_DIR/web/static/pmtiles/primorsky-krai-z15.pmtiles}"
CACHE_DIR="${MAP_CACHE_DIR:-$ROOT_DIR/.cache/map}"
TOOLS_IMAGE="${MAP_TOOLS_IMAGE:-hermes/map-tools:latest}"
PMTILES_IMAGE="${PMTILES_IMAGE:-ghcr.io/protomaps/go-pmtiles:latest}"
GDAL_IMAGE="${GDAL_IMAGE:-ghcr.io/osgeo/gdal:alpine-normal-latest}"
PBF_URL="${MAP_PBF_URL:-https://download.geofabrik.de/russia/far-eastern-fed-district-latest.osm.pbf}"
FAR_EAST_PBF="$CACHE_DIR/far-east.osm.pbf"
BOUNDARY_OSM="$CACHE_DIR/boundary-r${KRAI_RELATION_ID}.osm.pbf"
BOUNDARY_GEOJSON="$CACHE_DIR/boundary-r${KRAI_RELATION_ID}.geojson"
CLIPPED_PBF="$CACHE_DIR/primorsky.osm.pbf"
WORK_DIR="$CACHE_DIR/build"
COASTLINE_DIR="$CACHE_DIR/coastline"
COASTLINE_URL="https://osmdata.openstreetmap.de/download/water-polygons-split-4326.zip"
log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31mОШИБКА:\033[0m %s\n' "$*" >&2; exit 1; }
command -v docker >/dev/null 2>&1 || fail "нужен docker: без него не собрать тайлы"
if [ "$MAXZOOM" != "15" ]; then
	printf '\033[1;33mВНИМАНИЕ:\033[0m MAP_MAXZOOM=%s — проверьте, что MAX_ZOOM в process.lua совпадает с зумом конфига\n' "$MAXZOOM"
fi
build_tools() { # образ tilemaker + osmium: PBF-кроп и граница делает osmium в том же контейнере
  if docker image inspect "$TOOLS_IMAGE" >/dev/null 2>&1; then
    return
  fi
  log "Собираем образ $TOOLS_IMAGE (tilemaker + osmium)"
  # контекст — scripts/map (не корень репо: в него входит .cache на десятки ГБ)
  docker build -t "$TOOLS_IMAGE" -f "$PROFILE_DIR/Dockerfile.tools" "$PROFILE_DIR"
}
fetch_pbf() {
  if [ -f "$FAR_EAST_PBF" ]; then
    log "Экстракт уже скачан: $FAR_EAST_PBF"
    return
  fi
  log "Скачиваем экстракт Дальневосточного ФО (~400 MB, только при первом запуске)"
  curl -fL --retry 3 --retry-delay 2 -o "$FAR_EAST_PBF.part" "$PBF_URL"
  mv "$FAR_EAST_PBF.part" "$FAR_EAST_PBF"
}
extract_boundary() { # relation границы -> GeoJSON-полигон для кропа и береговой линии
  if [ -f "$BOUNDARY_GEOJSON" ] && [ "$BOUNDARY_GEOJSON" -nt "$FAR_EAST_PBF" ]; then
    log "Граница региона уже готова: $BOUNDARY_GEOJSON"
    return
  fi
  log "Достаём границу региона: relation $KRAI_RELATION_ID"
  docker run --rm -v "$CACHE_DIR:/data" --entrypoint osmium "$TOOLS_IMAGE" \
    getid -r "/data/$(basename "$FAR_EAST_PBF")" "r${KRAI_RELATION_ID}" \
    -o "/data/$(basename "$BOUNDARY_OSM")" --overwrite
  rm -f "$BOUNDARY_GEOJSON"
  docker run --rm -v "$CACHE_DIR:/data" --entrypoint osmium "$TOOLS_IMAGE" \
    export --geometry-types=polygon \
    "/data/$(basename "$BOUNDARY_OSM")" -o "/data/$(basename "$BOUNDARY_GEOJSON")"
  [ -s "$BOUNDARY_GEOJSON" ] || fail "не удалось построить полигон границы: $BOUNDARY_GEOJSON"
}
crop_pbf() { # кроп по границе региона, а не по bbox: иначе в архив попадут Китай, Хабаровский край и ЕАО
  if [ -f "$CLIPPED_PBF" ] && [ "$CLIPPED_PBF" -nt "$BOUNDARY_GEOJSON" ]; then
    log "Кроп уже готов: $CLIPPED_PBF"
    return
  fi
  log "Вырезаем регион по границе relation $KRAI_RELATION_ID"
  docker run --rm -v "$CACHE_DIR:/data" --entrypoint osmium "$TOOLS_IMAGE" \
    extract --polygon "/data/$(basename "$BOUNDARY_GEOJSON")" \
    "/data/$(basename "$FAR_EAST_PBF")" -o "/data/$(basename "$CLIPPED_PBF")" --overwrite
  [ -f "$CLIPPED_PBF" ] || fail "osmium не создал $CLIPPED_PBF"
}
geo_area() { # обрезает Water polygons до региона, чтобы ocean-слой не тянул всю планету
  if [ -f "$COASTLINE_DIR/water_polygons.shp" ] && [ "$COASTLINE_DIR/water_polygons.shp" -nt "$BOUNDARY_GEOJSON" ]; then
    log "Береговая линия уже подготовлена: $COASTLINE_DIR"
    return
  fi
  mkdir -p "$COASTLINE_DIR"
  local zip="$CACHE_DIR/water-polygons-4326.zip"
  if [ ! -f "$zip" ]; then
    log "Скачиваем береговую линию Osmdata (~870 MB, только при первой сборке)"
    curl -fL --retry 3 --retry-delay 2 -o "$zip" "$COASTLINE_URL"
  fi
  log "Распаковываем береговую линию"
  rm -rf "$CACHE_DIR/water-4326"
  unzip -o -q "$zip" -d "$CACHE_DIR/water-4326"
  log "Обрезаем береговую линию по границе региона"
  docker run --rm -v "$CACHE_DIR:/data" --entrypoint ogr2ogr "$GDAL_IMAGE" \
    -f "ESRI Shapefile" /data/coastline/water_polygons.shp \
    /data/water-4326/water-polygons-split-4326/water_polygons.shp \
    -clipsrc "/data/$(basename "$BOUNDARY_GEOJSON")"
  [ -f "$COASTLINE_DIR/water_polygons.shp" ] || fail "не удалось подготовить береговую линию"
}
render_tiles() {
  log "Рендерим векторные тайлы z0-$MAXZOOM (tilemaker)"
  rm -rf "$WORK_DIR"
  mkdir -p "$WORK_DIR/coastline"
  cp "$COASTLINE_DIR"/water_polygons.* "$WORK_DIR/coastline/"
  # Подставляем MAP_MAXZOOM в копию конфига (process.lua при нестандартном
  # зуме, помимо MAX_ZOOM, править не нужно — он читает зум через слои).
  local cfg="$CACHE_DIR/config-z${MAXZOOM}.json"
  sed -e 's/"maxzoom": 15/"maxzoom": '"$MAXZOOM"'/g' \
      -e 's/"basezoom": 15/"basezoom": '"$MAXZOOM"'/g' \
      -e 's/"combine_below": 15/"combine_below": '"$MAXZOOM"'/g' \
      "$PROFILE_DIR/config.json" > "$cfg"
  # Монтируем весь кэш: tilemaker читает PBF и конфиг оттуда и пишет результат в build/.
  docker run --rm \
    -v "$CACHE_DIR:/data" \
    -v "$PROFILE_DIR:/profile:ro" \
    -w /data/build \
    "$TOOLS_IMAGE" \
    --input /data/primorsky.osm.pbf \
    --output /data/build/region.mbtiles \
    --config "/data/$(basename "$cfg")" \
    --process /profile/process.lua \
    --threads "$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"
  [ -f "$WORK_DIR/region.mbtiles" ] || fail "tilemaker не создал MBTiles"
}
to_pmtiles() {
  log "Конвертируем MBTiles -> PMTiles"
  mkdir -p "$(dirname "$OUTPUT")"
  docker run --rm -v "$WORK_DIR:/data" -v "$(dirname "$OUTPUT"):/out" "$PMTILES_IMAGE" \
    convert /data/region.mbtiles "/out/$(basename "$OUTPUT")"
  [ -f "$OUTPUT" ] || fail "PMTiles не создан: $OUTPUT"
  log "Проверяем структуру архива"
  docker run --rm -v "$(dirname "$OUTPUT"):/out" "$PMTILES_IMAGE" verify "/out/$(basename "$OUTPUT")"
}
report() {
  local size_bytes size_human
  size_bytes=$(wc -c <"$OUTPUT" | tr -d ' ')
  size_human=$(du -h "$OUTPUT" | cut -f1)
  log "Готово: $OUTPUT ($size_human)"
  docker run --rm -v "$(dirname "$OUTPUT"):/out" "$PMTILES_IMAGE" show "/out/$(basename "$OUTPUT")" \
    | grep -E "bounds|min zoom|max zoom|addressed tiles"
  if [ "$size_bytes" -gt 524288000 ]; then
    printf '\033[1;33mВНИМАНИЕ:\033[0m файл больше 500 MB (%s).\n' "$size_human"
    cat <<'HINT'
Варианты уменьшения:
  - MAP_MAXZOOM=13 mise run map-download          # грубая карта всего края
  - два архива: z13 на весь край + z15 на агломерацию
    (область второго — через MAP_KRAI_RELATION_ID, например 1647394)
  - zstd: архив PMTiles не сжимается повторно, выигрыш даёт только LOD-схема
HINT
  fi
}
main() {
  mkdir -p "$CACHE_DIR"
  build_tools
  fetch_pbf
  extract_boundary
  crop_pbf
  geo_area
  render_tiles
  to_pmtiles
  report
}
main "$@"
